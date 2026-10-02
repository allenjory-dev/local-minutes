package adapters

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"scriberr/internal/transcription/interfaces"
)

func TestSecretOutputAcrossEveryChunkBoundary(t *testing.T) {
	const token = "hf_SYNTHETIC_TEST_CREDENTIAL"
	input := "before " + token + " after " + token + " trailing hf_"
	want := "before [REDACTED] after [REDACTED] trailing hf_"
	for chunk := 1; chunk <= len(input); chunk++ {
		var dst bytes.Buffer
		w := &secretOutput{dst: &dst, secret: token}
		for start := 0; start < len(input); start += chunk {
			end := min(start+chunk, len(input))
			if n, err := w.Write([]byte(input[start:end])); err != nil || n != end-start {
				t.Fatalf("write: %d %v", n, err)
			}
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if dst.String() != want {
			t.Fatalf("chunk %d: output was not sanitized", chunk)
		}
	}
}

type failedLogWriter struct{}

func (failedLogWriter) Write([]byte) (int, error) { return 0, errors.New("disk unavailable") }

func TestSecretOutputPreservesOrdinaryLogsAndErrors(t *testing.T) {
	var dst bytes.Buffer
	w := &secretOutput{dst: &dst}
	if _, err := w.Write([]byte("normal output\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil || dst.String() != "normal output\n" {
		t.Fatal("ordinary output changed")
	}
	w.dst = failedLogWriter{}
	if _, err := w.Write([]byte("output")); err == nil {
		t.Fatal("log failure hidden")
	}
}

func TestHFTokenEnvironmentDoesNotMutateParent(t *testing.T) {
	parent := []string{"PATH=tools", "HF_TOKEN=old", "hf_token=stale"}
	before := append([]string(nil), parent...)
	got := withHFToken(parent, "new")
	if !reflect.DeepEqual(got, []string{"PATH=tools", "HF_TOKEN=new"}) {
		t.Fatal("duplicate token environment")
	}
	if !reflect.DeepEqual(parent, before) {
		t.Fatal("parent environment changed")
	}
}

func TestHFTokenNeverAppearsInAdapterArguments(t *testing.T) {
	const token = "hf_SYNTHETIC_TEST_CREDENTIAL"
	t.Setenv("HF_TOKEN", token)
	input := interfaces.AudioInput{FilePath: "fixture.wav"}
	params := map[string]interface{}{"hf_token": token, "model": "tiny", "output_format": "json"}
	w := NewWhisperXAdapter(t.TempDir())
	p := NewPyAnnoteAdapter(t.TempDir())
	wa, err := w.buildWhisperXArgs(input, params, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pa, err := p.buildPyAnnoteArgs(input, params, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{wa, pa} {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, token) || strings.Contains(joined, "--hf_token") || strings.Contains(joined, "--hf-token") {
			t.Fatal("credential passed in command line")
		}
	}
}
