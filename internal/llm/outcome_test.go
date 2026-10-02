package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// privacyMarker stands in for transcript text; it must never reach logs.
const privacyMarker = "PRIVATE-TRANSCRIPT-MARKER-7f3a"

func ollamaLines(lines ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, l := range lines {
			_, _ = io.WriteString(w, l+"\n")
			w.(http.Flusher).Flush()
		}
	}
}

func collect(t *testing.T, s OutcomeStreamer, opts GenerationOptions) (string, StreamOutcome) {
	t.Helper()
	var b strings.Builder
	out := s.StreamWithOutcome(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hello"}}, opts, func(d string) error {
		b.WriteString(d)
		return nil
	})
	return b.String(), out
}

func TestOllamaStreamCompletedWithStop(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"Hel"},"done":false}`,
		`{"message":{"content":"lo"},"done":false}`,
		`{"message":{"content":""},"done":true,"done_reason":"stop","prompt_eval_count":12,"eval_count":2}`,
	))
	defer srv.Close()

	text, out := collect(t, NewOllamaService(srv.URL), GenerationOptions{})
	if text != "Hello" || out.Err != nil || !out.Completed || out.FinishReason != "stop" {
		t.Fatalf("unexpected result text=%q outcome=%+v", text, out)
	}
	if out.PromptTokens != 12 || out.OutputTokens != 2 {
		t.Fatalf("token counts not reported: %+v", out)
	}
}

func TestOllamaStreamReportsOutputLimit(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"{\"overview\":\"cut"},"done":false}`,
		`{"message":{"content":""},"done":true,"done_reason":"length"}`,
	))
	defer srv.Close()

	_, out := collect(t, NewOllamaService(srv.URL), GenerationOptions{})
	if !out.Completed || out.FinishReason != "length" {
		t.Fatalf("output limit must be reported as finish_reason length, got %+v", out)
	}
}

func TestOllamaStreamWithoutDoneIsIncomplete(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"partial"},"done":false}`,
	))
	defer srv.Close()

	text, out := collect(t, NewOllamaService(srv.URL), GenerationOptions{})
	if text != "partial" || out.Completed || out.Err != nil {
		t.Fatalf("a stream that ends without done=true must be incomplete, got text=%q %+v", text, out)
	}
}

func TestOllamaStreamErrorLineIsProviderError(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"partial"},"done":false}`,
		`{"error":"model runner has unexpectedly stopped"}`,
	))
	defer srv.Close()

	_, out := collect(t, NewOllamaService(srv.URL), GenerationOptions{})
	var pe *ProviderError
	if out.Completed || !errors.As(out.Err, &pe) {
		t.Fatalf("expected provider error, got %+v", out)
	}
	if pe.Message != "model runner has unexpectedly stopped" {
		t.Fatalf("unexpected provider message %q", pe.Message)
	}
}

func TestOllamaHTTPErrorKeepsMessageOutOfErrorString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"failed near: `+privacyMarker+`"}`)
	}))
	defer srv.Close()

	_, out := collect(t, NewOllamaService(srv.URL), GenerationOptions{})
	var pe *ProviderError
	if !errors.As(out.Err, &pe) || pe.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected HTTP provider error, got %+v", out)
	}
	if !strings.Contains(pe.Message, privacyMarker) {
		t.Fatalf("provider message should be available to callers, got %q", pe.Message)
	}
	if strings.Contains(out.Err.Error(), privacyMarker) {
		t.Fatal("Error() must not include provider text, which may echo request content")
	}
}

func TestOllamaStreamSkipsMalformedLinesButCountsThem(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"a"},"done":false}`,
		`not json`,
		`{"message":{"content":"b"},"done":true,"done_reason":"stop"}`,
	))
	defer srv.Close()

	text, out := collect(t, NewOllamaService(srv.URL), GenerationOptions{})
	if text != "ab" || !out.Completed || out.SkippedLines != 1 {
		t.Fatalf("unexpected text=%q outcome=%+v", text, out)
	}
}

func TestOllamaStreamCancellationIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"message":{"content":"first"},"done":false}`+"\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // hold the stream open until the client goes away
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := NewOllamaService(srv.URL).StreamWithOutcome(ctx, "m", []ChatMessage{{Role: "user", Content: "x"}}, GenerationOptions{}, func(string) error {
		cancel()
		return nil
	})
	if out.Completed || !errors.Is(out.Err, context.Canceled) {
		t.Fatalf("expected cancellation, got %+v", out)
	}
}

func TestOllamaStreamStopsWhenConsumerRejectsDelta(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"a"},"done":false}`,
		`{"message":{"content":"b"},"done":true,"done_reason":"stop"}`,
	))
	defer srv.Close()

	out := NewOllamaService(srv.URL).StreamWithOutcome(context.Background(), "m", nil, GenerationOptions{}, func(string) error {
		return errors.New("client went away")
	})
	if out.Completed || !errors.Is(out.Err, ErrDeltaRejected) {
		t.Fatalf("expected ErrDeltaRejected, got %+v", out)
	}
}

func TestOllamaRequestCarriesExplicitLimitsAndSchema(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		_, _ = io.WriteString(w, `{"message":{"content":"{}"},"done":true,"done_reason":"stop"}`+"\n")
	}))
	defer srv.Close()

	svc := NewOllamaService(srv.URL)
	schema := json.RawMessage(`{"type":"object"}`)
	collect(t, svc, GenerationOptions{MaxOutputTokens: 2048, ContextTokens: 8192, JSONSchema: schema})
	collect(t, svc, GenerationOptions{})

	options, ok := bodies[0]["options"].(map[string]any)
	if !ok || options["num_predict"] != float64(2048) || options["num_ctx"] != float64(8192) {
		t.Fatalf("explicit limits not sent: %v", bodies[0])
	}
	if _, hasTemp := options["temperature"]; hasTemp {
		t.Fatal("temperature must not override the model file when not requested")
	}
	if format, ok := bodies[0]["format"].(map[string]any); !ok || format["type"] != "object" {
		t.Fatalf("schema not sent as format: %v", bodies[0]["format"])
	}
	if _, has := bodies[1]["options"]; has {
		t.Fatalf("zero options must leave model defaults alone: %v", bodies[1])
	}
	if _, has := bodies[1]["format"]; has {
		t.Fatalf("format must be omitted when no schema is requested: %v", bodies[1])
	}
}

// captureOutput records both the standard logger and os.Stdout while fn runs.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	var logBuf bytes.Buffer
	prevLog := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevLog)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prevStdout := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	fn()
	os.Stdout = prevStdout
	_ = w.Close()
	stdout := <-done
	return logBuf.String() + string(stdout)
}

func TestOllamaStreamsDoNotLogRequestContent(t *testing.T) {
	srv := httptest.NewServer(ollamaLines(
		`{"message":{"content":"ok"},"done":true,"done_reason":"stop"}`,
	))
	defer srv.Close()
	svc := NewOllamaService(srv.URL)
	msgs := []ChatMessage{{Role: "user", Content: "Transcript: " + privacyMarker}}

	output := captureOutput(t, func() {
		svc.StreamWithOutcome(context.Background(), "m", msgs, GenerationOptions{}, func(string) error { return nil })
		// The legacy chat stream previously printed up to 2,000 characters of
		// the request body, including transcript text.
		content, errs := svc.ChatCompletionStream(context.Background(), "m", msgs, 0)
		for range content {
		}
		for range errs {
		}
	})
	if strings.Contains(output, privacyMarker) {
		t.Fatalf("request content reached logs/stdout: %q", output)
	}
}

func openAILines(lines ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			_, _ = io.WriteString(w, l+"\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

func TestOpenAIStreamCompletesOnDone(t *testing.T) {
	srv := httptest.NewServer(openAILines(
		`data: {"choices":[{"delta":{"content":"Hi"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	))
	defer srv.Close()

	text, out := collect(t, NewOpenAIService("k", &srv.URL), GenerationOptions{MaxOutputTokens: 64})
	if text != "Hi" || !out.Completed || out.FinishReason != "stop" || out.Err != nil {
		t.Fatalf("unexpected text=%q outcome=%+v", text, out)
	}
}

func TestOpenAIStreamReportsLength(t *testing.T) {
	srv := httptest.NewServer(openAILines(
		`data: {"choices":[{"delta":{"content":"cut"},"finish_reason":"length"}]}`,
		`data: [DONE]`,
	))
	defer srv.Close()

	_, out := collect(t, NewOpenAIService("k", &srv.URL), GenerationOptions{})
	if !out.Completed || out.FinishReason != "length" {
		t.Fatalf("expected length finish, got %+v", out)
	}
}

func TestOpenAIStreamWithoutDoneOrFinishIsIncomplete(t *testing.T) {
	srv := httptest.NewServer(openAILines(
		`data: {"choices":[{"delta":{"content":"partial"}}]}`,
	))
	defer srv.Close()

	_, out := collect(t, NewOpenAIService("k", &srv.URL), GenerationOptions{})
	if out.Completed || out.Err != nil {
		t.Fatalf("expected incomplete stream, got %+v", out)
	}
}

func TestOpenAIStreamErrorEventIsProviderError(t *testing.T) {
	srv := httptest.NewServer(openAILines(
		`data: {"error":{"message":"overloaded","code":"server_error"}}`,
	))
	defer srv.Close()

	_, out := collect(t, NewOpenAIService("k", &srv.URL), GenerationOptions{})
	var pe *ProviderError
	if out.Completed || !errors.As(out.Err, &pe) || pe.Message != "overloaded" {
		t.Fatalf("expected provider error, got %+v", out)
	}
}

func TestOpenAIHTTPErrorAndStreamingUnsupportedDetection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["max_tokens"] != float64(64) {
			t.Errorf("max_tokens not sent: %v", req)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"Unsupported value: 'stream' does not support true with this model.","type":"invalid_request_error","param":"stream","code":"unsupported_value"}}`)
	}))
	defer srv.Close()

	_, out := collect(t, NewOpenAIService("k", &srv.URL), GenerationOptions{MaxOutputTokens: 64})
	var pe *ProviderError
	if !errors.As(out.Err, &pe) || pe.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected HTTP provider error, got %+v", out)
	}
	if !IsStreamingUnsupported(out.Err) {
		t.Fatal("streaming-unsupported error not recognised")
	}
	if IsStreamingUnsupported(&ProviderError{StatusCode: 500, Message: "overloaded"}) {
		t.Fatal("ordinary provider errors must not trigger the non-streaming fallback")
	}
}

func TestUnreachableProviderIsReported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing is listening any more

	for name, svc := range map[string]OutcomeStreamer{
		"ollama": NewOllamaService(url),
		"openai": NewOpenAIService("k", &url),
	} {
		_, out := collect(t, svc, GenerationOptions{})
		if out.Completed || !errors.Is(out.Err, ErrProviderUnreachable) {
			t.Fatalf("%s: expected ErrProviderUnreachable, got %+v", name, out)
		}
	}
}
