package transcription

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"scriberr/internal/transcription/interfaces"
	"scriberr/internal/transcription/registry"
)

// countingAdapter records PrepareEnvironment calls so tests can prove which
// adapters a startup selection touched. It satisfies both TranscriptionAdapter
// and DiarizationAdapter.
type countingAdapter struct {
	modelID      string
	prepareErr   error
	prepareCalls int
}

func (c *countingAdapter) GetCapabilities() interfaces.ModelCapabilities {
	return interfaces.ModelCapabilities{ModelID: c.modelID, ModelFamily: "counting"}
}

func (c *countingAdapter) GetParameterSchema() []interfaces.ParameterSchema { return nil }

func (c *countingAdapter) ValidateParameters(map[string]interface{}) error { return nil }

func (c *countingAdapter) PrepareEnvironment(context.Context) error {
	c.prepareCalls++
	return c.prepareErr
}

func (c *countingAdapter) GetModelPath() string { return "/tmp/" + c.modelID }

func (c *countingAdapter) IsReady(context.Context) bool { return true }

func (c *countingAdapter) GetEstimatedProcessingTime(interfaces.AudioInput) time.Duration {
	return time.Second
}

func (c *countingAdapter) Transcribe(context.Context, interfaces.AudioInput, map[string]interface{}, interfaces.ProcessingContext) (*interfaces.TranscriptResult, error) {
	return &interfaces.TranscriptResult{}, nil
}

func (c *countingAdapter) GetSupportedModels() []string { return []string{c.modelID} }

func (c *countingAdapter) Diarize(context.Context, interfaces.AudioInput, map[string]interface{}, interfaces.ProcessingContext) (*interfaces.DiarizationResult, error) {
	return &interfaces.DiarizationResult{}, nil
}

func (c *countingAdapter) GetMaxSpeakers() int { return 10 }

func (c *countingAdapter) GetMinSpeakers() int { return 1 }

// newServiceWithCountingAdapters builds a service over a cleared registry that
// holds a whisperx-like transcription adapter and a pyannote-like diarization
// adapter.
func newServiceWithCountingAdapters(t *testing.T) (svc *UnifiedTranscriptionService, whisperx, pyannote *countingAdapter) {
	t.Helper()

	registry.ClearRegistry()
	t.Cleanup(registry.ClearRegistry)

	whisperx = &countingAdapter{modelID: ModelWhisperX}
	pyannote = &countingAdapter{modelID: ModelPyannote}

	registry.RegisterTranscriptionAdapter(ModelWhisperX, whisperx)
	registry.RegisterDiarizationAdapter(ModelPyannote, pyannote)

	return NewUnifiedTranscriptionService(new(MockJobRepository), t.TempDir(), t.TempDir()), whisperx, pyannote
}

func TestParseStartupModels(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "unset", raw: "", want: nil},
		{name: "blank is treated as unset", raw: "   ", want: nil},
		{name: "single id", raw: "whisperx", want: []string{"whisperx"}},
		{name: "trimmed list", raw: " whisperx , pyannote ", want: []string{"whisperx", "pyannote"}},
		{name: "deduplicated", raw: "whisperx,whisperx", want: []string{"whisperx"}},
		{name: "empty token", raw: "whisperx,,pyannote", wantErr: true},
		{name: "trailing separator", raw: "whisperx,", wantErr: true},
		{name: "separator only", raw: ",", wantErr: true},
		{name: "whitespace token", raw: "whisperx, ,pyannote", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStartupModels(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %v", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestInitializePreparesOnlySelectedModel(t *testing.T) {
	svc, whisperx, pyannote := newServiceWithCountingAdapters(t)
	t.Setenv(StartupModelsSetting, " whisperx , whisperx ")

	if err := svc.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if whisperx.prepareCalls != 1 {
		t.Errorf("expected whisperx prepared once, got %d calls", whisperx.prepareCalls)
	}
	// WhisperX covers diarization itself, so the separate pyannote environment
	// must stay untouched unless it is selected.
	if pyannote.prepareCalls != 0 {
		t.Errorf("expected pyannote not to be prepared, got %d calls", pyannote.prepareCalls)
	}
}

func TestInitializeFailsOnUnknownSelectedModel(t *testing.T) {
	svc, whisperx, pyannote := newServiceWithCountingAdapters(t)
	t.Setenv(StartupModelsSetting, "whisperx,sortformer")

	err := svc.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize to fail for an unregistered model id")
	}
	if !strings.Contains(err.Error(), "sortformer") {
		t.Errorf("error should name the unknown model, got: %v", err)
	}
	if whisperx.prepareCalls != 0 || pyannote.prepareCalls != 0 {
		t.Errorf("no model should be prepared for an invalid selection, got whisperx=%d pyannote=%d",
			whisperx.prepareCalls, pyannote.prepareCalls)
	}
}

func TestInitializeFailsOnEmptySelectedModelToken(t *testing.T) {
	svc, whisperx, _ := newServiceWithCountingAdapters(t)
	t.Setenv(StartupModelsSetting, "whisperx,,pyannote")

	if err := svc.Initialize(context.Background()); err == nil {
		t.Fatal("expected Initialize to fail for an empty model id")
	}
	if whisperx.prepareCalls != 0 {
		t.Errorf("no model should be prepared for an invalid selection, got %d calls", whisperx.prepareCalls)
	}
}

func TestInitializeReportsSelectedPreparationFailure(t *testing.T) {
	svc, whisperx, _ := newServiceWithCountingAdapters(t)
	whisperx.prepareErr = errors.New("whisperx environment missing")
	t.Setenv(StartupModelsSetting, "whisperx")

	err := svc.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize to report the preparation failure")
	}
	if !strings.Contains(err.Error(), "whisperx environment missing") {
		t.Errorf("error should wrap the adapter failure, got: %v", err)
	}

	// The failure must not be cached as a successful initialization.
	if err := svc.Initialize(context.Background()); err == nil {
		t.Fatal("expected the failure to repeat on retry")
	}
	if whisperx.prepareCalls != 2 {
		t.Errorf("expected the failed model to be retried, got %d calls", whisperx.prepareCalls)
	}
}

func TestInitializeWithoutSelectionPreparesAllModels(t *testing.T) {
	svc, whisperx, pyannote := newServiceWithCountingAdapters(t)
	// Blank value stands in for unconfigured, which keeps initialize-all.
	t.Setenv(StartupModelsSetting, "")

	if err := svc.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if whisperx.prepareCalls != 1 || pyannote.prepareCalls != 1 {
		t.Errorf("expected every registered model to be prepared, got whisperx=%d pyannote=%d",
			whisperx.prepareCalls, pyannote.prepareCalls)
	}
}
