package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"scriberr/internal/llm"
	"scriberr/internal/models"
	"scriberr/internal/summary"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// summaryGenerationTimeout bounds one generation (unchanged from upstream).
	summaryGenerationTimeout = 60 * time.Minute
	// maxSummaryOutputBytes guards memory against a misbehaving provider;
	// 2,048 output tokens are far below it.
	maxSummaryOutputBytes = 1 << 20
)

var errSummaryOutputTooLarge = errors.New("summary output exceeded the safety limit")

// attemptRegistry tracks attempts running in this process. An attempt stored
// as "running" but absent here was interrupted (for example by a restart).
type attemptRegistry struct {
	mu     sync.Mutex
	active map[string]struct{}
}

var activeSummaryAttempts = &attemptRegistry{active: map[string]struct{}{}}

func (r *attemptRegistry) add(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[id] = struct{}{}
}

func (r *attemptRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, id)
}

func (r *attemptRegistry) has(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.active[id]
	return ok
}

func newSummaryAttempt(transcriptionID string, templateID *string, mode, provider, model string, limits summary.Limits) *models.SummaryAttempt {
	return &models.SummaryAttempt{
		ID:               uuid.New().String(),
		TranscriptionID:  transcriptionID,
		TemplateID:       templateID,
		Mode:             mode,
		Provider:         provider,
		Model:            model,
		Status:           summary.StatusRunning,
		InputTokenBudget: limits.InputBudget(),
		ContextTokens:    limits.ContextTokens,
		MaxOutputTokens:  limits.MaxOutputTokens,
		StartedAt:        time.Now(),
	}
}

func inputTooLarge(check summary.InputCheck) summary.Classification {
	return summary.Classification{
		Status: summary.StatusRejected,
		Reason: summary.ReasonInputTooLarge,
		Detail: fmt.Sprintf("The transcript is too long for one local summary (estimated %d input tokens; the limit is %d). "+
			"Nothing was sent to the model and nothing was truncated. Long-meeting support is not implemented yet.",
			check.EstimatedTokens, check.BudgetTokens),
	}
}

func applyClassification(a *models.SummaryAttempt, cls summary.Classification) {
	now := time.Now()
	a.Status, a.Reason, a.Detail = cls.Status, cls.Reason, cls.Detail
	a.FinishedAt = &now
}

// recordRejectedAttempt stores an attempt that ended before any model call.
func (h *Handler) recordRejectedAttempt(a *models.SummaryAttempt, cls summary.Classification) {
	applyClassification(a, cls)
	if err := h.summaryRepo.CreateAttempt(context.Background(), a); err != nil {
		log.Printf("[summary] failed to record attempt=%s status=%s: %v", a.ID, a.Status, err)
	}
	logAttemptEnd(a)
}

// finishAttempt stores the final state of a started attempt. It uses a fresh
// context because the request context is already cancelled when the client
// went away.
func (h *Handler) finishAttempt(a *models.SummaryAttempt, cls summary.Classification) {
	applyClassification(a, cls)
	if err := h.summaryRepo.SaveAttempt(context.Background(), a); err != nil {
		log.Printf("[summary] failed to record attempt=%s status=%s: %v", a.ID, a.Status, err)
	}
}

// saveCompleted stores a completed summary and its attempt together. If that
// fails, the attempt is recorded as a storage failure and nothing else changes.
func (h *Handler) saveCompleted(s *models.Summary, a *models.SummaryAttempt) bool {
	applyClassification(a, summary.Classification{Status: summary.StatusCompleted})
	if err := h.summaryRepo.SaveCompletedSummary(context.Background(), s, a); err != nil {
		log.Printf("[summary] failed to save completed summary attempt=%s: %v", a.ID, err)
		a.SummaryID = nil
		h.finishAttempt(a, summary.Classification{
			Status: summary.StatusFailed,
			Reason: summary.ReasonStorageError,
			Detail: "The model finished, but the result could not be saved. The previous summary was kept.",
		})
		return false
	}
	return true
}

func classifyGeneration(out llm.StreamOutcome, output string, limits summary.Limits) summary.Classification {
	if errors.Is(out.Err, errSummaryOutputTooLarge) {
		return summary.Classification{
			Status: summary.StatusIncomplete,
			Reason: summary.ReasonOutputTooLarge,
			Detail: "The provider returned more output than the safety limit allows; generation was stopped.",
		}
	}
	return summary.ClassifyOutcome(out, output, limits)
}

func recordOutcomeMetrics(a *models.SummaryAttempt, out llm.StreamOutcome, outputBytes int) {
	a.FinishReason = out.FinishReason
	a.PromptTokens = out.PromptTokens
	a.OutputTokens = out.OutputTokens
	a.OutputChars = outputBytes
}

// reconcileStaleAttempt reports a "running" attempt that this process is not
// running as interrupted, and records that.
func (h *Handler) reconcileStaleAttempt(a *models.SummaryAttempt) *models.SummaryAttempt {
	if a.Status != summary.StatusRunning || activeSummaryAttempts.has(a.ID) {
		return a
	}
	h.finishAttempt(a, summary.Classification{
		Status: summary.StatusIncomplete,
		Reason: summary.ReasonInterrupted,
		Detail: "The server stopped before this attempt finished. Nothing from it was saved.",
	})
	return a
}

// SummaryView is the summary as presented to clients. DraftStatus and
// DraftNotice are set by the application for every summary, including those
// saved by older versions; model text cannot change them.
type SummaryView struct {
	ID                string                 `json:"id,omitempty"`
	TranscriptionID   string                 `json:"transcription_id"`
	TemplateID        *string                `json:"template_id"`
	Model             string                 `json:"model"`
	Provider          string                 `json:"provider"`
	Content           string                 `json:"content"`
	CreatedAt         *time.Time             `json:"created_at"`
	UpdatedAt         *time.Time             `json:"updated_at"`
	GenerationStatus  string                 `json:"generation_status"`
	Format            string                 `json:"format"`
	DraftStatus       string                 `json:"draft_status"`
	DraftNotice       string                 `json:"draft_notice"`
	Draft             *summary.StoredDraft   `json:"draft"`
	TranscriptChanged bool                   `json:"transcript_changed"`
	LatestAttempt     *models.SummaryAttempt `json:"latest_attempt"`
}

func fillSummaryView(v *SummaryView, s *models.Summary, job *models.TranscriptionJob) {
	created, updated := s.CreatedAt, s.UpdatedAt
	v.ID = s.ID
	v.TemplateID = s.TemplateID
	v.Model = s.Model
	v.Provider = s.Provider
	v.Content = s.Content
	v.CreatedAt, v.UpdatedAt = &created, &updated
	v.GenerationStatus = s.GenerationStatus
	v.Format = s.Format
	if v.GenerationStatus == "" {
		v.GenerationStatus = summary.StatusLegacy
	}
	if v.Format == "" {
		v.Format = summary.FormatLegacy
	}
	if v.GenerationStatus == summary.StatusLegacy {
		v.DraftNotice = summary.DraftNotice + " " + summary.LegacyNotice
	}
	if s.DraftJSON != nil && *s.DraftJSON != "" {
		var d summary.StoredDraft
		if err := json.Unmarshal([]byte(*s.DraftJSON), &d); err == nil {
			v.Draft = &d
		} else {
			log.Printf("[summary] stored draft for summary=%s could not be read: %v", s.ID, err)
		}
	}
	if s.TranscriptSHA256 != "" {
		v.TranscriptChanged = job == nil || job.Transcript == nil || summary.Fingerprint(*job.Transcript) != s.TranscriptSHA256
	}
}

// GroundedSummaryRequest selects the model for an evidence-linked draft.
type GroundedSummaryRequest struct {
	Model      string  `json:"model" binding:"required"`
	TemplateID *string `json:"template_id,omitempty"`
}

// GenerateGroundedSummary creates an evidence-linked draft from the stored transcript.
// @Summary Generate an evidence-linked summary draft
// @Description Builds the model input from the stored transcript (segment IDs and speaker labels), requests JSON candidates, and checks every segment reference, quote and speaker against the transcript. Speakers and timestamps are taken from the transcript. The result is saved only if generation completed and the output parsed; every item needs review and technical claims stay UNVERIFIED. Oversized transcripts are rejected (413) without calling the model.
// @Tags summarize
// @Accept json
// @Produce json
// @Param id path string true "Transcription ID"
// @Param request body GroundedSummaryRequest true "Model and optional template"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 413 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Failure 502 {object} map[string]interface{}
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /api/v1/transcription/{id}/summary/grounded [post]
func (h *Handler) GenerateGroundedSummary(c *gin.Context) {
	tid := c.Param("id")
	var req GroundedSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()

	job, err := h.jobRepo.FindByID(ctx, tid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transcription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load transcription"})
		return
	}
	if job.Status != models.StatusCompleted || job.Transcript == nil || strings.TrimSpace(*job.Transcript) == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "An evidence-linked draft needs a completed transcript"})
		return
	}
	limits, err := summary.LimitsFromEnv(os.Getenv)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid summary limit configuration: " + err.Error()})
		return
	}
	svc, provider, err := h.getLLMService(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	streamer, ok := svc.(llm.OutcomeStreamer)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "The configured provider cannot report whether generation completed"})
		return
	}

	attempt := newSummaryAttempt(tid, req.TemplateID, summary.ModeGrounded, provider, req.Model, limits)
	_, prevErr := h.summaryRepo.GetLatestSummary(ctx, tid)
	previousKept := prevErr == nil || (job.Summary != nil && *job.Summary != "")

	transcript, err := summary.ParseStoredTranscript(*job.Transcript)
	if err != nil {
		h.recordRejectedAttempt(attempt, summary.Classification{
			Status: summary.StatusRejected,
			Reason: summary.ReasonNoSegments,
			Detail: "The stored transcript has no timed segments with text, so nothing can be cited as evidence.",
		})
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": attempt.Detail, "attempt": attempt, "previous_summary_kept": previousKept})
		return
	}
	messages := summary.BuildGroundedMessages(transcript)
	check := limits.CheckInput(messages)
	attempt.InputTokensEstimate = check.EstimatedTokens
	if !check.Fits {
		h.recordRejectedAttempt(attempt, inputTooLarge(check))
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": attempt.Detail, "attempt": attempt, "previous_summary_kept": previousKept})
		return
	}

	activeSummaryAttempts.add(attempt.ID)
	defer activeSummaryAttempts.remove(attempt.ID)
	if err := h.summaryRepo.CreateAttempt(ctx, attempt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record summary attempt"})
		return
	}
	logAttemptStart(attempt, check, len(transcript.Segments))

	genCtx, cancel := context.WithTimeout(ctx, summaryGenerationTimeout)
	defer cancel()
	opts := limits.GenerationOptions()
	opts.JSONSchema = summary.GroundedOutputSchema()
	var output strings.Builder
	outcome := streamer.StreamWithOutcome(genCtx, req.Model, messages, opts, func(delta string) error {
		if output.Len()+len(delta) > maxSummaryOutputBytes {
			return errSummaryOutputTooLarge
		}
		output.WriteString(delta)
		return nil
	})
	recordOutcomeMetrics(attempt, outcome, output.Len())
	cls := classifyGeneration(outcome, output.String(), limits)

	var draft *summary.StoredDraft
	if cls.Completed() {
		draft, err = summary.BuildDraft(output.String(), transcript)
		if err != nil {
			cls = summary.Classification{
				Status: summary.StatusFailed,
				Reason: summary.ReasonMalformedOutput,
				Detail: "The model output was not a valid evidence-linked draft (malformed JSON or missing fields). Nothing was saved.",
			}
		}
	}
	if !cls.Completed() {
		h.finishAttempt(attempt, cls)
		logAttemptEnd(attempt)
		c.JSON(cls.HTTPStatus(), gin.H{"error": attempt.Detail, "attempt": attempt, "previous_summary_kept": previousKept})
		return
	}

	draftJSON, err := json.Marshal(draft)
	if err != nil {
		h.finishAttempt(attempt, summary.Classification{Status: summary.StatusFailed, Reason: summary.ReasonStorageError, Detail: "The draft could not be encoded for storage."})
		c.JSON(http.StatusInternalServerError, gin.H{"error": attempt.Detail, "attempt": attempt, "previous_summary_kept": previousKept})
		return
	}
	stored := string(draftJSON)
	saved := &models.Summary{
		TranscriptionID: tid,
		TemplateID:      req.TemplateID,
		Model:           req.Model,
		Provider:        provider,
		Content: summary.RenderMarkdown(draft, summary.RenderMeta{
			Model: req.Model, Provider: provider, GeneratedAt: time.Now(),
		}),
		GenerationStatus: summary.StatusCompleted,
		Format:           summary.ModeGrounded,
		AttemptID:        &attempt.ID,
		TranscriptSHA256: transcript.SHA256,
		DraftJSON:        &stored,
	}
	if !h.saveCompleted(saved, attempt) {
		logAttemptEnd(attempt)
		c.JSON(http.StatusInternalServerError, gin.H{"error": attempt.Detail, "attempt": attempt, "previous_summary_kept": previousKept})
		return
	}
	logAttemptEnd(attempt)

	view := &SummaryView{TranscriptionID: tid, DraftStatus: summary.DraftStatus, DraftNotice: summary.DraftNotice, LatestAttempt: attempt}
	fillSummaryView(view, saved, job)
	c.JSON(http.StatusCreated, gin.H{"attempt": attempt, "summary": view})
}

// GetSummaryAttempt returns one generation attempt.
// @Summary Get a summary generation attempt
// @Description Reports how a generation ended (completed, failed, incomplete, cancelled or rejected) and why. Clients streaming a free-form summary use it to learn whether the text was saved.
// @Tags summarize
// @Produce json
// @Param id path string true "Transcription ID"
// @Param attempt_id path string true "Attempt ID"
// @Success 200 {object} models.SummaryAttempt
// @Failure 404 {object} map[string]string
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /api/v1/transcription/{id}/summary/attempts/{attempt_id} [get]
func (h *Handler) GetSummaryAttempt(c *gin.Context) {
	attempt, err := h.summaryRepo.GetAttempt(c.Request.Context(), c.Param("attempt_id"))
	if err != nil || attempt.TranscriptionID != c.Param("id") {
		if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Summary attempt not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary attempt"})
		return
	}
	c.JSON(http.StatusOK, h.reconcileStaleAttempt(attempt))
}
