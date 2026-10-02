package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"scriberr/internal/llm"
	"scriberr/internal/models"
	"scriberr/internal/summary"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SummarizeRequest struct {
	Model           string  `json:"model" binding:"required"`
	Content         string  `json:"content" binding:"required"`
	TranscriptionID string  `json:"transcription_id" binding:"required"`
	TemplateID      *string `json:"template_id,omitempty"`
}

// summaryAttemptHeader carries the attempt ID of a streamed free-form summary.
// The stream itself cannot report failure after the 200 status is sent, so
// clients must read the attempt to learn whether the text was saved.
const summaryAttemptHeader = "X-Summary-Attempt-Id"

// Summarize streams a free-form LLM summary for client-supplied content.
// @Summary Summarize content (free-form, no evidence checks)
// @Description Streams model text. The response header X-Summary-Attempt-Id identifies the attempt; fetch /api/v1/transcription/{id}/summary/attempts/{attempt_id} after the stream ends. Text is saved only if the provider confirms completion; failed, cancelled, truncated or empty generations are recorded as attempts and never replace a saved summary.
// @Tags summarize
// @Accept json
// @Produce text/plain
// @Param request body SummarizeRequest true "Summarize request"
// @Success 200 {string} string "Streamed model text"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 413 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /api/v1/summarize [post]
func (h *Handler) Summarize(c *gin.Context) {
	var req SummarizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()

	if _, err := h.jobRepo.FindByID(ctx, req.TranscriptionID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transcription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load transcription"})
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

	messages := []llm.ChatMessage{{Role: "user", Content: req.Content}}
	attempt := newSummaryAttempt(req.TranscriptionID, req.TemplateID, summary.ModeFreeform, provider, req.Model, limits)
	check := limits.CheckInput(messages)
	attempt.InputTokensEstimate = check.EstimatedTokens
	if !check.Fits {
		h.recordRejectedAttempt(attempt, inputTooLarge(check))
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": attempt.Detail, "attempt": attempt})
		return
	}

	activeSummaryAttempts.add(attempt.ID)
	defer activeSummaryAttempts.remove(attempt.ID)
	if err := h.summaryRepo.CreateAttempt(ctx, attempt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record summary attempt"})
		return
	}
	logAttemptStart(attempt, check, 0)

	c.Header(summaryAttemptHeader, attempt.ID)
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering
	c.Status(http.StatusOK)

	genCtx, cancel := context.WithTimeout(ctx, summaryGenerationTimeout)
	defer cancel()
	var output strings.Builder
	write := func(delta string) error {
		if output.Len()+len(delta) > maxSummaryOutputBytes {
			return errSummaryOutputTooLarge
		}
		output.WriteString(delta)
		if _, err := c.Writer.WriteString(delta); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	outcome := streamer.StreamWithOutcome(genCtx, req.Model, messages, limits.GenerationOptions(), write)

	// Upstream fallback for OpenAI models/organisations that cannot stream.
	if llm.IsStreamingUnsupported(outcome.Err) && output.Len() == 0 {
		outcome = nonStreamingFallback(genCtx, svc, req.Model, messages, write)
	}

	cls := classifyGeneration(outcome, output.String(), limits)
	recordOutcomeMetrics(attempt, outcome, output.Len())
	if !cls.Completed() {
		h.finishAttempt(attempt, cls)
		logAttemptEnd(attempt)
		if !c.Writer.Written() {
			// Nothing was streamed yet, so a real error status can still be sent.
			for _, k := range []string{"Content-Type", "Transfer-Encoding", "Cache-Control", "Connection", "X-Accel-Buffering"} {
				c.Writer.Header().Del(k)
			}
			c.JSON(cls.HTTPStatus(), gin.H{"error": attempt.Detail, "attempt": attempt})
		}
		return
	}

	saved := &models.Summary{
		TranscriptionID:  req.TranscriptionID,
		TemplateID:       req.TemplateID,
		Model:            req.Model,
		Provider:         provider,
		Content:          output.String(),
		GenerationStatus: summary.StatusCompleted,
		Format:           summary.ModeFreeform,
		AttemptID:        &attempt.ID,
	}
	h.saveCompleted(saved, attempt)
	logAttemptEnd(attempt)
}

// nonStreamingFallback runs one non-streaming completion and converts it into
// an outcome. Only OpenAI reports a finish reason here.
func nonStreamingFallback(ctx context.Context, svc llm.Service, model string, messages []llm.ChatMessage, write func(string) error) llm.StreamOutcome {
	resp, err := svc.ChatCompletion(ctx, model, messages, 0.0)
	if err != nil {
		if ctx.Err() != nil {
			return llm.StreamOutcome{Err: ctx.Err()}
		}
		// The upstream error text includes the provider body; keep it out.
		return llm.StreamOutcome{Err: &llm.ProviderError{Message: "non-streaming fallback request failed"}}
	}
	out := llm.StreamOutcome{Completed: true}
	if resp == nil || len(resp.Choices) == 0 {
		return out
	}
	out.FinishReason = resp.Choices[0].FinishReason
	out.PromptTokens = resp.Usage.PromptTokens
	out.OutputTokens = resp.Usage.CompletionTokens
	if content := resp.Choices[0].Message.Content; content != "" {
		if err := write(content); err != nil {
			return llm.StreamOutcome{Err: errors.Join(llm.ErrDeltaRejected, err)}
		}
	}
	return out
}

// GetSummaryForTranscription returns the latest saved summary with its
// application-assigned status and the latest generation attempt.
// @Summary Get latest summary for transcription
// @Description Returns the most recent saved summary (or an empty one), always labelled as an unverified AI draft, with generation status, evidence-linked draft data when available, whether the transcript changed since generation, and the most recent generation attempt (which may have failed without replacing the saved summary).
// @Tags summarize
// @Produce json
// @Param id path string true "Transcription ID"
// @Success 200 {object} SummaryView
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /api/v1/transcription/{id}/summary [get]
func (h *Handler) GetSummaryForTranscription(c *gin.Context) {
	tid := c.Param("id")
	if tid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Transcription ID required"})
		return
	}
	ctx := c.Request.Context()
	view := &SummaryView{
		TranscriptionID:  tid,
		GenerationStatus: summary.StatusNone,
		DraftStatus:      summary.DraftStatus,
		DraftNotice:      summary.DraftNotice,
	}

	job, jobErr := h.jobRepo.FindByID(ctx, tid)
	if jobErr != nil && !errors.Is(jobErr, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary"})
		return
	}
	if jobErr != nil {
		job = nil
	}

	s, err := h.summaryRepo.GetLatestSummary(ctx, tid)
	switch {
	case err == nil:
		fillSummaryView(view, s, job)
	case errors.Is(err, gorm.ErrRecordNotFound):
		// Fallback: an older version may have cached a summary on the job only.
		if job != nil && job.Summary != nil && *job.Summary != "" {
			updated := job.UpdatedAt
			view.Content = *job.Summary
			view.CreatedAt = &updated
			view.UpdatedAt = &updated
			view.GenerationStatus = summary.StatusLegacy
			view.Format = summary.FormatLegacy
			view.DraftNotice = summary.DraftNotice + " " + summary.LegacyNotice
		}
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary"})
		return
	}

	if attempt, err := h.summaryRepo.GetLatestAttempt(ctx, tid); err == nil {
		view.LatestAttempt = h.reconcileStaleAttempt(attempt)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary attempts"})
		return
	}
	c.JSON(http.StatusOK, view)
}

func logAttemptStart(a *models.SummaryAttempt, check summary.InputCheck, segments int) {
	// Metadata only: never transcript text, prompts, model output or provider messages.
	log.Printf("[summary] start attempt=%s transcription=%s mode=%s provider=%s model=%s est_input_tokens=%d budget=%d segments=%d",
		a.ID, a.TranscriptionID, a.Mode, a.Provider, a.Model, check.EstimatedTokens, check.BudgetTokens, segments)
}

func logAttemptEnd(a *models.SummaryAttempt) {
	duration := int64(0)
	if a.FinishedAt != nil {
		duration = a.FinishedAt.Sub(a.StartedAt).Milliseconds()
	}
	log.Printf("[summary] end attempt=%s transcription=%s mode=%s status=%s reason=%s finish_reason=%s output_chars=%d prompt_tokens=%d output_tokens=%d duration_ms=%d",
		a.ID, a.TranscriptionID, a.Mode, a.Status, a.Reason, a.FinishReason, a.OutputChars, a.PromptTokens, a.OutputTokens, duration)
}
