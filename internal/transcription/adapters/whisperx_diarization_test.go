package adapters

import (
	"scriberr/internal/transcription/interfaces"
	"testing"
)

func TestWhisperXDiarizationModelSelection(t *testing.T) {
	const community = "pyannote/speaker-diarization-community-1"
	for _, tc := range []struct{ name, selected, want string }{
		{"default", "", community},
		{"ui_alias", "pyannote", community},
		{"explicit_community", community, community},
		{"explicit_legacy", "pyannote/speaker-diarization-3.1", "pyannote/speaker-diarization-3.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := NewWhisperXAdapter(t.TempDir())
			params := map[string]interface{}{"diarize": true}
			if tc.selected != "" {
				params["diarize_model"] = tc.selected
			}
			if err := adapter.ValidateParameters(params); err != nil {
				t.Fatal(err)
			}
			args, err := adapter.buildWhisperXArgs(interfaces.AudioInput{FilePath: "fixture.wav"}, params, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			for i, arg := range args {
				if arg == "--diarize_model" && i+1 < len(args) {
					if args[i+1] != tc.want {
						t.Fatalf("model=%q, want %q", args[i+1], tc.want)
					}
					return
				}
			}
			t.Fatal("missing diarization model argument")
		})
	}
}
