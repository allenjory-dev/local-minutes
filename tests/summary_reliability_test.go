package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"scriberr/internal/api"
	"scriberr/internal/auth"
	"scriberr/internal/config"
	"scriberr/internal/database"
	"scriberr/internal/models"
	"scriberr/internal/processing"
	"scriberr/internal/queue"
	"scriberr/internal/repository"
	"scriberr/internal/service"
	"scriberr/internal/sse"
	"scriberr/internal/summary"
	"scriberr/internal/transcription"
	"scriberr/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// These tests drive the real handlers, repositories, SQLite schema and Ollama
// client against a scripted local HTTP server that imitates Ollama's NDJSON
// stream. No model runs; every model response is a fixed synthetic string.

const summaryTestMarker = "CONFIDENTIAL-MEETING-MARKER-91c2"

// scriptedOllama imitates /api/chat with a replaceable response script.
type scriptedOllama struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	respond  http.HandlerFunc
}

func newScriptedOllama() *scriptedOllama {
	s := &scriptedOllama{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.requests = append(s.requests, body)
		respond := s.respond
		s.mu.Unlock()
		respond(w, r)
	}))
	return s
}

func (s *scriptedOllama) set(h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.respond = h
	s.requests = nil
}

func (s *scriptedOllama) calls() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.requests...)
}

func ndjsonChunk(content string) string {
	b, _ := json.Marshal(map[string]any{"message": map[string]string{"role": "assistant", "content": content}, "done": false})
	return string(b)
}

func writeLines(w http.ResponseWriter, lines []string) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, l := range lines {
		_, _ = io.WriteString(w, l+"\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// streamAnswer splits text into small chunks and ends with the given done line.
func streamAnswer(text, doneLine string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var lines []string
		for len(text) > 0 {
			n := 7
			if len(text) < n {
				n = len(text)
			}
			lines = append(lines, ndjsonChunk(text[:n]))
			text = text[n:]
		}
		if doneLine != "" {
			lines = append(lines, doneLine)
		}
		writeLines(w, lines)
	}
}

const doneStop = `{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":900,"eval_count":250}`

func loadSummaryFixture(t *testing.T, file, scenario string) (transcriptJSON, modelOutput string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "internal", "summary", "testdata", file))
	require.NoError(t, err)
	var f struct {
		Transcript   json.RawMessage            `json:"transcript"`
		ModelOutputs map[string]json.RawMessage `json:"model_outputs"`
	}
	require.NoError(t, json.Unmarshal(b, &f))
	if scenario != "" {
		out, ok := f.ModelOutputs[scenario]
		require.True(t, ok, scenario)
		modelOutput = string(out)
	}
	return string(f.Transcript), modelOutput
}

type SummaryReliabilityTestSuite struct {
	suite.Suite
	helper *TestHelper
	router *gin.Engine
	ollama *scriptedOllama
}

func buildSummaryTestRouter(t *testing.T, helper *TestHelper, db *gorm.DB) *gin.Engine {
	jobRepo := repository.NewJobRepository(db)
	unified := transcription.NewUnifiedJobProcessor(jobRepo, helper.Config.TempDir, helper.Config.TranscriptsDir)
	quick, err := transcription.NewQuickTranscriptionService(helper.Config, unified, jobRepo)
	require.NoError(t, err)
	handler := api.NewHandler(
		helper.Config,
		helper.AuthService,
		service.NewUserService(repository.NewUserRepository(db), helper.AuthService),
		service.NewFileService(),
		jobRepo,
		repository.NewAPIKeyRepository(db),
		repository.NewProfileRepository(db),
		repository.NewUserRepository(db),
		repository.NewLLMConfigRepository(db),
		repository.NewSummaryRepository(db),
		repository.NewChatRepository(db),
		repository.NewNoteRepository(db),
		repository.NewSpeakerMappingRepository(db),
		repository.NewRefreshTokenRepository(db),
		queue.NewTaskQueue(1, unified, jobRepo),
		unified,
		quick,
		processing.NewMultiTrackProcessor(db, jobRepo),
		sse.NewBroadcaster(),
	)
	return api.SetupRoutes(handler, helper.AuthService)
}

func (s *SummaryReliabilityTestSuite) SetupSuite() {
	s.helper = NewTestHelper(s.T(), "summary_reliability_test.db")
	s.ollama = newScriptedOllama()
	s.router = buildSummaryTestRouter(s.T(), s.helper, s.helper.DB)
}

func (s *SummaryReliabilityTestSuite) TearDownSuite() {
	s.ollama.srv.Close()
	s.helper.Cleanup()
}

func (s *SummaryReliabilityTestSuite) SetupTest() {
	db := s.helper.DB.Session(&gorm.Session{AllowGlobalUpdate: true})
	require.NoError(s.T(), db.Unscoped().Delete(&models.SummaryAttempt{}).Error)
	require.NoError(s.T(), db.Unscoped().Delete(&models.Summary{}).Error)
	require.NoError(s.T(), db.Unscoped().Delete(&models.LLMConfig{}).Error)
	require.NoError(s.T(), db.Unscoped().Delete(&models.TranscriptionJob{}).Error)
	base := s.ollama.srv.URL
	require.NoError(s.T(), s.helper.DB.Create(&models.LLMConfig{Provider: "ollama", BaseURL: &base, IsActive: true}).Error)
	s.ollama.set(streamAnswer("", ""))
}

func TestSummaryReliabilityTestSuite(t *testing.T) {
	suite.Run(t, new(SummaryReliabilityTestSuite))
}

func (s *SummaryReliabilityTestSuite) createJob(transcriptJSON string) *models.TranscriptionJob {
	job := s.helper.CreateTestTranscriptionJob(s.T(), "Synthetic meeting")
	job.Status = models.StatusCompleted
	job.Transcript = &transcriptJSON
	require.NoError(s.T(), s.helper.DB.Save(job).Error)
	return job
}

func (s *SummaryReliabilityTestSuite) do(ctx context.Context, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, reader)
	require.NoError(s.T(), err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.helper.TestToken)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

type groundedResponse struct {
	Error               string                `json:"error"`
	Attempt             models.SummaryAttempt `json:"attempt"`
	Summary             *api.SummaryView      `json:"summary"`
	PreviousSummaryKept bool                  `json:"previous_summary_kept"`
}

func (s *SummaryReliabilityTestSuite) generate(jobID string) (int, groundedResponse) {
	w := s.do(context.Background(), http.MethodPost, "/api/v1/transcription/"+jobID+"/summary/grounded",
		map[string]any{"model": "local-minutes-summary:7b"})
	var resp groundedResponse
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	return w.Code, resp
}

func (s *SummaryReliabilityTestSuite) getSummary(jobID string) api.SummaryView {
	w := s.do(context.Background(), http.MethodGet, "/api/v1/transcription/"+jobID+"/summary", nil)
	require.Equal(s.T(), http.StatusOK, w.Code, w.Body.String())
	var v api.SummaryView
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &v))
	return v
}

func (s *SummaryReliabilityTestSuite) countSummaries(jobID string) int64 {
	var n int64
	require.NoError(s.T(), s.helper.DB.Model(&models.Summary{}).Where("transcription_id = ?", jobID).Count(&n).Error)
	return n
}

func (s *SummaryReliabilityTestSuite) TestGroundedDraftIsSavedWithTranscriptEvidence() {
	t := s.T()
	transcriptJSON, output := loadSummaryFixture(t, "supplier_commitment.json", "correct")
	job := s.createJob(transcriptJSON)
	s.ollama.set(streamAnswer(output, doneStop))

	code, resp := s.generate(job.ID)
	require.Equal(t, http.StatusCreated, code, resp.Error)
	assert.Equal(t, summary.StatusCompleted, resp.Attempt.Status)
	require.NotNil(t, resp.Summary)
	require.NotNil(t, resp.Attempt.SummaryID)
	assert.Equal(t, resp.Summary.ID, *resp.Attempt.SummaryID)

	view := s.getSummary(job.ID)
	assert.Equal(t, summary.StatusCompleted, view.GenerationStatus)
	assert.Equal(t, summary.ModeGrounded, view.Format)
	assert.Equal(t, summary.DraftStatus, view.DraftStatus)
	assert.Equal(t, summary.DraftNotice, view.DraftNotice)
	assert.True(t, strings.HasPrefix(view.Content, "> **"+summary.DraftNotice))
	assert.False(t, view.TranscriptChanged)
	require.NotNil(t, view.Draft)
	require.Len(t, view.Draft.Candidates, 2)
	c := view.Draft.Candidates[0]
	assert.Equal(t, summary.KindCommitment, c.Kind)
	assert.Equal(t, summary.ReviewNeedsReview, c.ReviewState)
	require.Len(t, c.Evidence, 1)
	assert.Equal(t, "S2", c.Evidence[0].SegmentID)
	assert.Equal(t, 4.2, c.Evidence[0].Start)
	assert.Equal(t, "SPEAKER_00", c.Evidence[0].Speaker)
	require.NotNil(t, view.LatestAttempt)
	assert.Equal(t, summary.StatusCompleted, view.LatestAttempt.Status)
	assert.Equal(t, 900, view.LatestAttempt.PromptTokens)

	// The job cache holds the same app-labelled text for older readers.
	var stored models.TranscriptionJob
	require.NoError(t, s.helper.DB.First(&stored, "id = ?", job.ID).Error)
	require.NotNil(t, stored.Summary)
	assert.Equal(t, view.Content, *stored.Summary)

	// The provider received explicit limits, the schema and a separate system message.
	calls := s.ollama.calls()
	require.Len(t, calls, 1)
	opts := calls[0]["options"].(map[string]any)
	assert.Equal(t, float64(summary.DefaultContextTokens), opts["num_ctx"])
	assert.Equal(t, float64(summary.DefaultMaxOutputTokens), opts["num_predict"])
	assert.NotNil(t, calls[0]["format"])
	msgs := calls[0]["messages"].([]any)
	require.Len(t, msgs, 2)
	assert.Equal(t, "system", msgs[0].(map[string]any)["role"])
	assert.Contains(t, msgs[1].(map[string]any)["content"], "[S2] SPEAKER_00: I will call the supplier by Friday")
}

func (s *SummaryReliabilityTestSuite) TestFailedGenerationsNeverReplaceTheSavedSummary() {
	t := s.T()
	transcriptJSON, good := loadSummaryFixture(t, "supplier_commitment.json", "correct")
	job := s.createJob(transcriptJSON)
	s.ollama.set(streamAnswer(good, doneStop))
	code, first := s.generate(job.ID)
	require.Equal(t, http.StatusCreated, code)
	firstID := first.Summary.ID
	firstContent := first.Summary.Content

	partial := good[:len(good)/2]
	cases := []struct {
		name     string
		script   http.HandlerFunc
		httpCode int
		status   string
		reason   string
	}{
		{"provider HTTP error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"model runner crashed"}`)
		}, http.StatusBadGateway, summary.StatusFailed, summary.ReasonProviderError},
		{"error inside the stream", streamAnswer(partial, `{"error":"model runner has unexpectedly stopped"}`),
			http.StatusBadGateway, summary.StatusFailed, summary.ReasonProviderError},
		{"output budget exhausted", streamAnswer(partial, `{"message":{"content":""},"done":true,"done_reason":"length"}`),
			http.StatusBadGateway, summary.StatusIncomplete, summary.ReasonOutputLimit},
		{"stream ended without done", streamAnswer(partial, ""),
			http.StatusBadGateway, summary.StatusIncomplete, summary.ReasonStreamEnded},
		{"empty output", streamAnswer("", doneStop),
			http.StatusBadGateway, summary.StatusFailed, summary.ReasonEmptyOutput},
		{"malformed output", streamAnswer("## Executive summary\nThe meeting went well.", doneStop),
			http.StatusBadGateway, summary.StatusFailed, summary.ReasonMalformedOutput},
		{"context overflow reported by provider", streamAnswer(good, `{"message":{"content":""},"done":true,"done_reason":"stop","prompt_eval_count":8000,"eval_count":10}`),
			http.StatusBadGateway, summary.StatusIncomplete, summary.ReasonContextExceeded},
	}
	for _, tc := range cases {
		s.ollama.set(tc.script)
		code, resp := s.generate(job.ID)
		assert.Equal(t, tc.httpCode, code, tc.name)
		assert.Equal(t, tc.status, resp.Attempt.Status, tc.name)
		assert.Equal(t, tc.reason, resp.Attempt.Reason, tc.name)
		assert.NotEmpty(t, resp.Attempt.Detail, tc.name)
		assert.Nil(t, resp.Attempt.SummaryID, tc.name)
		assert.True(t, resp.PreviousSummaryKept, tc.name)

		view := s.getSummary(job.ID)
		assert.Equal(t, firstID, view.ID, "%s: the previous summary stays current", tc.name)
		assert.Equal(t, firstContent, view.Content, tc.name)
		require.NotNil(t, view.LatestAttempt, tc.name)
		assert.Equal(t, resp.Attempt.ID, view.LatestAttempt.ID, tc.name)
		assert.Equal(t, tc.status, view.LatestAttempt.Status, tc.name)
		assert.Equal(t, int64(1), s.countSummaries(job.ID), tc.name)
	}
	var stored models.TranscriptionJob
	require.NoError(t, s.helper.DB.First(&stored, "id = ?", job.ID).Error)
	assert.Equal(t, firstContent, *stored.Summary, "the job cache is not overwritten by failures")
}

func (s *SummaryReliabilityTestSuite) TestProviderUnreachableIsAFailure() {
	t := s.T()
	transcriptJSON, _ := loadSummaryFixture(t, "no_actions.json", "")
	job := s.createJob(transcriptJSON)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	require.NoError(t, s.helper.DB.Model(&models.LLMConfig{}).Where("is_active = ?", true).Update("base_url", deadURL).Error)

	code, resp := s.generate(job.ID)
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Equal(t, summary.ReasonProviderUnreachable, resp.Attempt.Reason)
	assert.False(t, resp.PreviousSummaryKept)
	assert.Equal(t, summary.StatusNone, s.getSummary(job.ID).GenerationStatus)
}

func (s *SummaryReliabilityTestSuite) TestCancelledGenerationIsRecordedAndNotSaved() {
	t := s.T()
	transcriptJSON, good := loadSummaryFixture(t, "supplier_commitment.json", "correct")
	job := s.createJob(transcriptJSON)
	firstChunk := make(chan struct{})
	s.ollama.set(func(w http.ResponseWriter, r *http.Request) {
		writeLines(w, []string{ndjsonChunk(good[:20])})
		close(firstChunk)
		<-r.Context().Done() // keep generating until the app gives up
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder)
	go func() {
		done <- s.do(ctx, http.MethodPost, "/api/v1/transcription/"+job.ID+"/summary/grounded", map[string]any{"model": "m"})
	}()
	select {
	case <-firstChunk:
	case <-time.After(10 * time.Second):
		t.Fatal("provider never received the request")
	}
	cancel() // the browser closed the dialog or lost the connection
	var w *httptest.ResponseRecorder
	select {
	case w = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not stop after cancellation")
	}
	assert.Equal(t, 499, w.Code)

	view := s.getSummary(job.ID)
	assert.Equal(t, summary.StatusNone, view.GenerationStatus)
	require.NotNil(t, view.LatestAttempt)
	assert.Equal(t, summary.StatusCancelled, view.LatestAttempt.Status)
	assert.Equal(t, int64(0), s.countSummaries(job.ID))
}

func (s *SummaryReliabilityTestSuite) TestOversizedTranscriptIsRejectedWithoutCallingTheModel() {
	t := s.T()
	transcriptJSON, good := loadSummaryFixture(t, "supplier_commitment.json", "correct")
	job := s.createJob(transcriptJSON)
	s.ollama.set(streamAnswer(good, doneStop))
	code, first := s.generate(job.ID)
	require.Equal(t, http.StatusCreated, code)

	var segs []map[string]any
	for i := 0; i < 600; i++ {
		segs = append(segs, map[string]any{"start": float64(i) * 5, "end": float64(i)*5 + 4.5, "speaker": fmt.Sprintf("SPEAKER_0%d", i%3),
			"text": "We reviewed the venting layout, the appliance clearances and the inspection schedule again in detail."})
	}
	long, _ := json.Marshal(map[string]any{"text": "long", "segments": segs})
	require.NoError(t, s.helper.DB.Model(&models.TranscriptionJob{}).Where("id = ?", job.ID).Update("transcript", string(long)).Error)

	s.ollama.set(streamAnswer(good, doneStop))
	code, resp := s.generate(job.ID)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
	assert.Equal(t, summary.StatusRejected, resp.Attempt.Status)
	assert.Equal(t, summary.ReasonInputTooLarge, resp.Attempt.Reason)
	assert.Greater(t, resp.Attempt.InputTokensEstimate, resp.Attempt.InputTokenBudget)
	assert.Contains(t, resp.Attempt.Detail, "nothing was truncated")
	assert.Empty(t, s.ollama.calls(), "the model is never called with an oversized transcript")

	view := s.getSummary(job.ID)
	assert.Equal(t, first.Summary.ID, view.ID)
	assert.True(t, view.TranscriptChanged, "the saved draft no longer matches the stored transcript")
}

func (s *SummaryReliabilityTestSuite) TestTranscriptWithoutSegmentsIsRejected() {
	t := s.T()
	job := s.createJob(`{"text":"Plain text without timed segments."}`)
	code, resp := s.generate(job.ID)
	assert.Equal(t, http.StatusUnprocessableEntity, code)
	assert.Equal(t, summary.ReasonNoSegments, resp.Attempt.Reason)
	assert.Empty(t, s.ollama.calls())
}

func (s *SummaryReliabilityTestSuite) TestTechnicalClaimsStayUnverifiedThroughTheAPI() {
	t := s.T()
	transcriptJSON, output := loadSummaryFixture(t, "conflicting_technical_claims.json", "model_claims_verified")
	job := s.createJob(transcriptJSON)
	s.ollama.set(streamAnswer(output, doneStop))
	code, _ := s.generate(job.ID)
	require.Equal(t, http.StatusCreated, code)

	w := s.do(context.Background(), http.MethodGet, "/api/v1/transcription/"+job.ID+"/summary", nil)
	body := w.Body.String()
	assert.NotContains(t, strings.ReplaceAll(body, "UNVERIFIED", ""), "VERIFIED")
	assert.NotContains(t, body, `"verified":`)
	view := s.getSummary(job.ID)
	for _, c := range view.Draft.Candidates {
		assert.Equal(t, summary.VerificationUnverified, c.VerificationStatus, c.ID)
		assert.Equal(t, summary.ReviewNeedsReview, c.ReviewState, c.ID)
	}
}

func (s *SummaryReliabilityTestSuite) TestInjectedInstructionsAreFlaggedNotObeyed() {
	t := s.T()
	transcriptJSON, output := loadSummaryFixture(t, "prompt_injection.json", "obeys_injection")
	job := s.createJob(transcriptJSON)
	s.ollama.set(streamAnswer(output, doneStop))
	code, resp := s.generate(job.ID)
	require.Equal(t, http.StatusCreated, code)

	c := resp.Summary.Draft.Candidates[0]
	var codes []string
	for _, f := range c.Flags {
		codes = append(codes, f.Code)
	}
	assert.Contains(t, codes, summary.FlagInstructionLikeText)
	assert.Contains(t, codes, summary.FlagOwnerNotSupported)
	assert.Equal(t, summary.ReviewNeedsReview, c.ReviewState)

	// The injected text reached the model only inside the transcript data block.
	msgs := s.ollama.calls()[0]["messages"].([]any)
	system := msgs[0].(map[string]any)["content"].(string)
	user := msgs[1].(map[string]any)["content"].(string)
	assert.NotContains(t, system, "ignore all previous instructions")
	assert.True(t, strings.HasPrefix(user, "<transcript>"))
}

func (s *SummaryReliabilityTestSuite) TestConversationWithNoActionsIsAValidDraft() {
	t := s.T()
	transcriptJSON, output := loadSummaryFixture(t, "no_actions.json", "correct")
	job := s.createJob(transcriptJSON)
	s.ollama.set(streamAnswer(output, doneStop))
	code, resp := s.generate(job.ID)
	require.Equal(t, http.StatusCreated, code)
	assert.Empty(t, resp.Summary.Draft.Candidates)
	assert.Contains(t, resp.Summary.Content, "_None extracted._")
}

func (s *SummaryReliabilityTestSuite) TestLegacySummariesRemainUnverifiedDrafts() {
	t := s.T()
	job := s.createJob(`{"segments":[{"start":0,"end":1,"text":"hi","speaker":"SPEAKER_00"}]}`)
	// Insert exactly the columns an older binary writes; defaults fill the rest.
	require.NoError(t, s.helper.DB.Exec(
		"INSERT INTO summaries (id, transcription_id, template_id, model, content, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, ?, ?)",
		"legacy-summary-1", job.ID, "local-minutes-summary:7b", "## Action items\n- Task: buy tester; Owner: SPEAKER_00", time.Now(), time.Now()).Error)

	view := s.getSummary(job.ID)
	assert.Equal(t, summary.StatusLegacy, view.GenerationStatus)
	assert.Equal(t, summary.FormatLegacy, view.Format)
	assert.Equal(t, summary.DraftStatus, view.DraftStatus)
	assert.Contains(t, view.DraftNotice, "UNVERIFIED AI DRAFT")
	assert.Contains(t, view.DraftNotice, "may be partial")
	assert.Nil(t, view.Draft)

	// A summary cached only on the job record (older fallback path).
	job2 := s.createJob(`{"segments":[{"start":0,"end":1,"text":"hi","speaker":"SPEAKER_00"}]}`)
	cached := "Old cached summary"
	require.NoError(t, s.helper.DB.Model(&models.TranscriptionJob{}).Where("id = ?", job2.ID).Update("summary", cached).Error)
	view = s.getSummary(job2.ID)
	assert.Equal(t, cached, view.Content)
	assert.Equal(t, summary.StatusLegacy, view.GenerationStatus)
	assert.Contains(t, view.DraftNotice, "UNVERIFIED AI DRAFT")
}

func (s *SummaryReliabilityTestSuite) TestFreeformStreamSavesOnlyConfirmedCompleteOutput() {
	t := s.T()
	transcriptJSON, _ := loadSummaryFixture(t, "no_actions.json", "")
	job := s.createJob(transcriptJSON)
	stream := func(body any) (*httptest.ResponseRecorder, models.SummaryAttempt) {
		w := s.do(context.Background(), http.MethodPost, "/api/v1/summarize/", body)
		id := w.Header().Get("X-Summary-Attempt-Id")
		require.NotEmpty(t, id, w.Body.String())
		aw := s.do(context.Background(), http.MethodGet, "/api/v1/transcription/"+job.ID+"/summary/attempts/"+id, nil)
		require.Equal(t, http.StatusOK, aw.Code)
		var a models.SummaryAttempt
		require.NoError(t, json.Unmarshal(aw.Body.Bytes(), &a))
		return w, a
	}
	req := map[string]any{"model": "m", "content": "Transcript:\nhello\n\nInstructions:\nSummarize.", "transcription_id": job.ID}

	// Many small chunks then done: every chunk is streamed and saved (the old
	// handler could drop buffered chunks when its error channel closed first).
	var parts []string
	for i := 0; i < 400; i++ {
		parts = append(parts, fmt.Sprintf("w%03d ", i))
	}
	full := strings.Join(parts, "")
	s.ollama.set(func(w http.ResponseWriter, r *http.Request) {
		var lines []string
		for _, p := range parts {
			lines = append(lines, ndjsonChunk(p))
		}
		writeLines(w, append(lines, doneStop))
	})
	w, a := stream(req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, full, w.Body.String())
	assert.Equal(t, summary.StatusCompleted, a.Status)
	view := s.getSummary(job.ID)
	assert.Equal(t, full, view.Content)
	assert.Equal(t, summary.ModeFreeform, view.Format)
	assert.Equal(t, summary.DraftNotice, view.DraftNotice)
	savedID := view.ID

	for name, script := range map[string]http.HandlerFunc{
		"provider error mid-stream": streamAnswer("partial text", `{"error":"out of memory"}`),
		"output limit":              streamAnswer("partial text", `{"message":{"content":""},"done":true,"done_reason":"length"}`),
		"no done":                   streamAnswer("partial text", ""),
	} {
		s.ollama.set(script)
		w, a := stream(req)
		assert.Equal(t, "partial text", w.Body.String(), name)
		assert.NotEqual(t, summary.StatusCompleted, a.Status, name)
		assert.Nil(t, a.SummaryID, name)
		assert.Equal(t, savedID, s.getSummary(job.ID).ID, "%s: partial text is never saved", name)
	}

	s.ollama.set(streamAnswer("x", doneStop))
	huge := map[string]any{"model": "m", "content": strings.Repeat("venting clearance discussion ", 4000), "transcription_id": job.ID}
	w = s.do(context.Background(), http.MethodPost, "/api/v1/summarize/", huge)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Empty(t, s.ollama.calls())
	assert.Equal(t, int64(1), s.countSummaries(job.ID))
}

func (s *SummaryReliabilityTestSuite) TestSummaryLogsContainNoTranscriptOrModelText() {
	t := s.T()
	transcriptJSON, output := loadSummaryFixture(t, "supplier_commitment.json", "correct")
	transcriptJSON = strings.Replace(transcriptJSON, "Sounds good.", "Sounds good. "+summaryTestMarker, 1)
	job := s.createJob(transcriptJSON)
	output = strings.Replace(output, "were discussed.", "were discussed. "+summaryTestMarker, 1)

	captured := captureAllOutput(t, func() {
		s.ollama.set(streamAnswer(output, doneStop))
		code, _ := s.generate(job.ID)
		require.Equal(t, http.StatusCreated, code)

		s.ollama.set(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"bad input near `+summaryTestMarker+`"}`)
		})
		code, resp := s.generate(job.ID)
		require.Equal(t, http.StatusBadGateway, code)
		assert.Contains(t, resp.Attempt.Detail, summaryTestMarker, "the provider message is shown to the user, not logged")

		s.ollama.set(streamAnswer("Free-form "+summaryTestMarker, doneStop))
		s.do(context.Background(), http.MethodPost, "/api/v1/summarize/",
			map[string]any{"model": "m", "content": "Transcript: " + summaryTestMarker, "transcription_id": job.ID})
	})
	assert.Contains(t, captured, "[summary] start", "operational metadata is still logged")
	assert.NotContains(t, captured, summaryTestMarker, "transcript, prompt, model output and provider text stay out of logs")
}

// captureAllOutput records the standard logger, the application logger and
// os.Stdout while fn runs.
func captureAllOutput(t *testing.T, fn func()) string {
	t.Helper()
	var logBuf bytes.Buffer
	prevLog := log.Writer()
	log.SetOutput(&logBuf)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	prevStdout := os.Stdout
	os.Stdout = w
	logger.Init("debug") // the app logger binds os.Stdout at init
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	defer func() {
		os.Stdout = prevStdout
		logger.Init(os.Getenv("LOG_LEVEL"))
		log.SetOutput(prevLog)
	}()
	fn()
	_ = w.Close()
	return logBuf.String() + string(<-done)
}

// legacySummaryRow mirrors the summaries model before this change (same gorm
// tags, including the foreign key), so the migration test starts from the
// schema an existing installation actually has.
type legacySummaryRow struct {
	ID              string                  `gorm:"primaryKey;type:varchar(36)"`
	TranscriptionID string                  `gorm:"type:varchar(36);index;not null"`
	TemplateID      *string                 `gorm:"type:varchar(36)"`
	Model           string                  `gorm:"type:varchar(255);not null"`
	Content         string                  `gorm:"type:text;not null"`
	CreatedAt       time.Time               `gorm:"autoCreateTime"`
	UpdatedAt       time.Time               `gorm:"autoUpdateTime"`
	Transcription   models.TranscriptionJob `gorm:"foreignKey:TranscriptionID;constraint:OnDelete:CASCADE"`
}

func (legacySummaryRow) TableName() string { return "summaries" }

// TestSummaryPersistsAcrossRestartAndMigration starts from a database with the
// pre-change summaries schema and one saved summary, migrates it, generates,
// "restarts" (closes and reopens the database with fresh handlers) and reopens.
func TestSummaryPersistsAcrossRestartAndMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "restart.db")
	helper := &TestHelper{
		Config:      &config.Config{DatabasePath: dbPath, JWTSecret: "test-secret-key-for-unit-tests", UploadDir: filepath.Join(dir, "uploads")},
		AuthService: auth.NewAuthService("test-secret-key-for-unit-tests"),
	}

	// 1. The database as the previous binary left it: old summaries table and
	// one summary, no attempts table.
	require.NoError(t, database.Initialize(dbPath))
	require.NoError(t, database.DB.Migrator().DropTable(&models.SummaryAttempt{}, &models.Summary{}))
	require.NoError(t, database.DB.AutoMigrate(&legacySummaryRow{}))
	require.False(t, database.DB.Migrator().HasColumn(&models.Summary{}, "generation_status"))
	transcriptJSON, output := loadSummaryFixture(t, "deadline_correction.json", "corrected_deadline")
	title := "Restart test"
	job := &models.TranscriptionJob{Title: &title, Status: models.StatusCompleted, AudioPath: "synthetic.wav", Transcript: &transcriptJSON}
	require.NoError(t, database.DB.Create(job).Error)
	require.NoError(t, database.DB.Create(&legacySummaryRow{ID: "legacy-1", TranscriptionID: job.ID, Model: "old", Content: "legacy text"}).Error)
	require.NoError(t, database.Close())

	// 2. Start the new code on that file: AutoMigrate adds columns and a table.
	require.NoError(t, database.Initialize(dbPath))
	defer func() { _ = database.Close() }()
	helper.DB = database.DB
	helper.createTestCredentials(t)
	require.True(t, helper.DB.Migrator().HasColumn(&models.Summary{}, "generation_status"))
	require.True(t, helper.DB.Migrator().HasTable(&models.SummaryAttempt{}))
	var migrated models.Summary
	require.NoError(t, helper.DB.First(&migrated, "id = ?", "legacy-1").Error)
	assert.Equal(t, "legacy text", migrated.Content, "existing summaries survive the migration")
	assert.Equal(t, summary.StatusLegacy, migrated.GenerationStatus)
	assert.Equal(t, summary.FormatLegacy, migrated.Format)

	ollama := newScriptedOllama()
	defer ollama.srv.Close()
	base := ollama.srv.URL
	require.NoError(t, helper.DB.Create(&models.LLMConfig{Provider: "ollama", BaseURL: &base, IsActive: true}).Error)

	call := func(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reader = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+helper.TestToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	router := buildSummaryTestRouter(t, helper, helper.DB)

	var legacyView api.SummaryView
	require.NoError(t, json.Unmarshal(call(router, http.MethodGet, "/api/v1/transcription/"+job.ID+"/summary", nil).Body.Bytes(), &legacyView))
	assert.Equal(t, "legacy-1", legacyView.ID)
	assert.Equal(t, summary.StatusLegacy, legacyView.GenerationStatus, "pre-existing rows are labelled legacy")
	assert.Contains(t, legacyView.DraftNotice, "UNVERIFIED AI DRAFT")

	ollama.set(streamAnswer(output, doneStop))
	w := call(router, http.MethodPost, "/api/v1/transcription/"+job.ID+"/summary/grounded", map[string]any{"model": "m"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var before api.SummaryView
	require.NoError(t, json.Unmarshal(call(router, http.MethodGet, "/api/v1/transcription/"+job.ID+"/summary", nil).Body.Bytes(), &before))

	// A failed attempt afterwards must not hide the saved draft after restart.
	ollama.set(streamAnswer("{\"overview\":", `{"message":{"content":""},"done":true,"done_reason":"length"}`))
	require.Equal(t, http.StatusBadGateway, call(router, http.MethodPost, "/api/v1/transcription/"+job.ID+"/summary/grounded", map[string]any{"model": "m"}).Code)
	// An attempt left "running" by a crash.
	require.NoError(t, helper.DB.Create(&models.SummaryAttempt{TranscriptionID: job.ID, Mode: summary.ModeGrounded, Status: summary.StatusRunning,
		StartedAt: time.Now(), CreatedAt: time.Now().Add(time.Minute)}).Error)

	// 3. Restart: close the database and build everything again from disk.
	require.NoError(t, database.Close())
	require.NoError(t, database.Initialize(dbPath))
	helper.DB = database.DB
	router = buildSummaryTestRouter(t, helper, database.DB)

	var after api.SummaryView
	require.NoError(t, json.Unmarshal(call(router, http.MethodGet, "/api/v1/transcription/"+job.ID+"/summary", nil).Body.Bytes(), &after))
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.Content, after.Content)
	assert.Equal(t, summary.StatusCompleted, after.GenerationStatus)
	require.NotNil(t, after.Draft)
	assert.Equal(t, before.Draft.Candidates, after.Draft.Candidates)
	assert.Equal(t, "Tuesday", after.Draft.Candidates[0].Due)
	require.NotNil(t, after.LatestAttempt)
	assert.Equal(t, summary.StatusIncomplete, after.LatestAttempt.Status)
	assert.Equal(t, summary.ReasonInterrupted, after.LatestAttempt.Reason, "a running attempt from before the restart is reported as interrupted")

	var n int64
	require.NoError(t, database.DB.Model(&models.Summary{}).Where("transcription_id = ?", job.ID).Count(&n).Error)
	assert.Equal(t, int64(2), n, "the legacy row and the new draft are both kept")
}
