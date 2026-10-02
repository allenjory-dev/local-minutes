package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"scriberr/internal/transcription/interfaces"
)

// fakeAdapter counts PrepareEnvironment calls and can fail on demand. It
// satisfies both TranscriptionAdapter and DiarizationAdapter so a single type
// can stand in for either registration.
type fakeAdapter struct {
	modelID      string
	prepareErr   error
	prepareCalls int
}

func (f *fakeAdapter) GetCapabilities() interfaces.ModelCapabilities {
	return interfaces.ModelCapabilities{ModelID: f.modelID, ModelFamily: "fake"}
}

func (f *fakeAdapter) GetParameterSchema() []interfaces.ParameterSchema { return nil }

func (f *fakeAdapter) ValidateParameters(map[string]interface{}) error { return nil }

func (f *fakeAdapter) PrepareEnvironment(context.Context) error {
	f.prepareCalls++
	return f.prepareErr
}

func (f *fakeAdapter) GetModelPath() string { return "/tmp/" + f.modelID }

func (f *fakeAdapter) IsReady(context.Context) bool { return f.prepareCalls > 0 && f.prepareErr == nil }

func (f *fakeAdapter) GetEstimatedProcessingTime(interfaces.AudioInput) time.Duration {
	return time.Second
}

func (f *fakeAdapter) Transcribe(context.Context, interfaces.AudioInput, map[string]interface{}, interfaces.ProcessingContext) (*interfaces.TranscriptResult, error) {
	return &interfaces.TranscriptResult{}, nil
}

func (f *fakeAdapter) GetSupportedModels() []string { return []string{f.modelID} }

func (f *fakeAdapter) Diarize(context.Context, interfaces.AudioInput, map[string]interface{}, interfaces.ProcessingContext) (*interfaces.DiarizationResult, error) {
	return &interfaces.DiarizationResult{}, nil
}

func (f *fakeAdapter) GetMaxSpeakers() int { return 10 }

func (f *fakeAdapter) GetMinSpeakers() int { return 1 }

// registerFakes installs a whisperx-like transcription adapter and a
// pyannote-like diarization adapter in a cleared registry.
func registerFakes(t *testing.T) (reg *ModelRegistry, whisperx, pyannote *fakeAdapter) {
	t.Helper()

	ClearRegistry()
	t.Cleanup(ClearRegistry)

	whisperx = &fakeAdapter{modelID: "whisperx"}
	pyannote = &fakeAdapter{modelID: "pyannote"}

	RegisterTranscriptionAdapter("whisperx", whisperx)
	RegisterDiarizationAdapter("pyannote", pyannote)

	return GetRegistry(), whisperx, pyannote
}

func TestInitializeSelectedModelsPreparesOnlySelected(t *testing.T) {
	reg, whisperx, pyannote := registerFakes(t)

	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"}); err != nil {
		t.Fatalf("InitializeSelectedModels failed: %v", err)
	}

	if whisperx.prepareCalls != 1 {
		t.Errorf("expected whisperx prepared once, got %d calls", whisperx.prepareCalls)
	}
	if pyannote.prepareCalls != 0 {
		t.Errorf("expected pyannote not to be prepared, got %d calls", pyannote.prepareCalls)
	}
}

func TestInitializeSelectedModelsValidatesBeforePreparing(t *testing.T) {
	reg, whisperx, pyannote := registerFakes(t)

	err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx", "not_a_model"})
	if err == nil {
		t.Fatal("expected an error for an unknown model id")
	}
	if !strings.Contains(err.Error(), "not_a_model") {
		t.Errorf("error should name the unknown model, got: %v", err)
	}
	if whisperx.prepareCalls != 0 || pyannote.prepareCalls != 0 {
		t.Errorf("validation must run before any preparation, got whisperx=%d pyannote=%d",
			whisperx.prepareCalls, pyannote.prepareCalls)
	}
}

func TestInitializeSelectedModelsRejectsEmptyAndMissingSelection(t *testing.T) {
	reg, whisperx, _ := registerFakes(t)

	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx", "   "}); err == nil {
		t.Error("expected an error for an empty model id")
	}
	if err := reg.InitializeSelectedModels(context.Background(), nil); err == nil {
		t.Error("expected an error for an empty selection")
	}
	if whisperx.prepareCalls != 0 {
		t.Errorf("expected no preparation for an invalid selection, got %d calls", whisperx.prepareCalls)
	}
}

func TestInitializeSelectedModelsTrimsAndDeduplicates(t *testing.T) {
	reg, whisperx, _ := registerFakes(t)

	if err := reg.InitializeSelectedModels(context.Background(), []string{" whisperx ", "whisperx"}); err != nil {
		t.Fatalf("InitializeSelectedModels failed: %v", err)
	}

	if whisperx.prepareCalls != 1 {
		t.Errorf("expected a deduplicated single preparation, got %d calls", whisperx.prepareCalls)
	}
}

func TestInitializeSelectedModelsPropagatesFailureAndRetries(t *testing.T) {
	reg, whisperx, _ := registerFakes(t)
	whisperx.prepareErr = errors.New("missing dependency")

	err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"})
	if err == nil {
		t.Fatal("expected preparation failure to be returned, not swallowed")
	}
	if !strings.Contains(err.Error(), "missing dependency") {
		t.Errorf("error should wrap the adapter failure, got: %v", err)
	}

	// A failed model must not be recorded as prepared.
	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"}); err == nil {
		t.Fatal("expected the failure to repeat on retry")
	}
	if whisperx.prepareCalls != 2 {
		t.Errorf("expected the failed model to be retried, got %d calls", whisperx.prepareCalls)
	}

	// Once preparation succeeds it is remembered and not repeated.
	whisperx.prepareErr = nil
	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"}); err != nil {
		t.Fatalf("retry after fixing the environment failed: %v", err)
	}
	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"}); err != nil {
		t.Fatalf("repeat initialization failed: %v", err)
	}
	if whisperx.prepareCalls != 3 {
		t.Errorf("expected a successful model to be prepared once, got %d calls", whisperx.prepareCalls)
	}
}

func TestInitializeSelectedModelsPreparesNewSelection(t *testing.T) {
	reg, whisperx, pyannote := registerFakes(t)

	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"}); err != nil {
		t.Fatalf("first initialization failed: %v", err)
	}
	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx", "pyannote"}); err != nil {
		t.Fatalf("second initialization failed: %v", err)
	}

	if whisperx.prepareCalls != 1 {
		t.Errorf("expected whisperx to stay prepared once, got %d calls", whisperx.prepareCalls)
	}
	if pyannote.prepareCalls != 1 {
		t.Errorf("expected the newly selected model to be prepared, got %d calls", pyannote.prepareCalls)
	}
}

func TestInitializeSelectedModelsDoesNotSuppressInitializeAll(t *testing.T) {
	reg, whisperx, pyannote := registerFakes(t)

	if err := reg.InitializeSelectedModels(context.Background(), []string{"whisperx"}); err != nil {
		t.Fatalf("selected initialization failed: %v", err)
	}
	if err := reg.InitializeModels(context.Background()); err != nil {
		t.Fatalf("InitializeModels failed: %v", err)
	}

	if pyannote.prepareCalls != 1 {
		t.Errorf("initialize-all should still prepare unselected models, got %d calls", pyannote.prepareCalls)
	}
	if whisperx.prepareCalls == 0 {
		t.Error("initialize-all should still prepare every registered model")
	}
}
