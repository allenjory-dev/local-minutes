package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// GenerationOptions bounds a single generation request. Zero values leave the
// provider or model defaults in place.
type GenerationOptions struct {
	// Temperature is sent only when greater than zero, matching the existing
	// chat behaviour, so model-file defaults (for example a pinned seed and
	// temperature) stay in force.
	Temperature float64
	// MaxOutputTokens caps generated tokens (Ollama num_predict, OpenAI max_tokens).
	MaxOutputTokens int
	// ContextTokens sets the runtime context window where the provider allows
	// it (Ollama num_ctx). Sending it keeps the application's input budget and
	// the runtime configuration in agreement.
	ContextTokens int
	// JSONSchema requests schema-constrained output where supported (Ollama
	// "format"). Providers without support ignore it; callers must still
	// validate the output.
	JSONSchema json.RawMessage
}

// StreamOutcome reports how a streamed generation ended. A stream is only a
// candidate for success when Err is nil and Completed is true; FinishReason
// must still be checked for truncation (for example "length").
type StreamOutcome struct {
	// Completed is true only when the provider explicitly signalled the end of
	// generation (Ollama done=true, OpenAI [DONE] or a finish_reason).
	Completed bool
	// FinishReason is the provider-reported reason, for example "stop" or
	// "length". Empty when the provider did not report one.
	FinishReason string
	// PromptTokens and OutputTokens are provider-reported counts, 0 if unknown.
	PromptTokens int
	OutputTokens int
	// SkippedLines counts stream lines that could not be parsed.
	SkippedLines int
	// Err is a transport, HTTP, provider or context error.
	Err error
}

// OutcomeStreamer is implemented by providers that can report how a streamed
// generation ended. onDelta receives output in order; returning an error from
// it stops the stream and the outcome's Err wraps ErrDeltaRejected.
type OutcomeStreamer interface {
	StreamWithOutcome(ctx context.Context, model string, messages []ChatMessage, opts GenerationOptions, onDelta func(string) error) StreamOutcome
}

// ErrDeltaRejected wraps an error returned by an onDelta callback, for example
// when the client connection has gone away.
var ErrDeltaRejected = errors.New("stream consumer stopped accepting output")

// ProviderError is an error reported by the model provider, either as an HTTP
// status or inside the stream. Error() deliberately omits Message: providers
// may echo request content, so the message must not reach application logs.
type ProviderError struct {
	StatusCode int
	Message    string
	// Code and Param are OpenAI-style error fields, empty when not reported.
	Code  string
	Param string
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("provider returned HTTP %d", e.StatusCode)
	}
	return "provider reported an error during generation"
}

const (
	maxProviderMessageRunes = 500
	maxErrorBodyBytes       = 16 << 10
	maxStreamLineBytes      = 4 << 20
)

// boundedProviderMessage keeps a provider error short and printable.
func boundedProviderMessage(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(s))
	if utf8.RuneCountInString(s) > maxProviderMessageRunes {
		s = string([]rune(s)[:maxProviderMessageRunes]) + "…"
	}
	return s
}

// providerErrorFromBody parses {"error":"..."} (Ollama) or
// {"error":{"message":"...","code":"...","param":"..."}} (OpenAI) and falls
// back to the bounded body text.
func providerErrorFromBody(statusCode int, body []byte) *ProviderError {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		pe := providerErrorFromField(envelope.Error)
		pe.StatusCode = statusCode
		return pe
	}
	return &ProviderError{StatusCode: statusCode, Message: boundedProviderMessage(string(body))}
}

// providerErrorFromField parses the value of an "error" field.
func providerErrorFromField(raw json.RawMessage) *ProviderError {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return &ProviderError{Message: boundedProviderMessage(s)}
	}
	var obj struct {
		Message string          `json:"message"`
		Code    json.RawMessage `json:"code"`
		Param   string          `json:"param"`
	}
	if json.Unmarshal(raw, &obj) == nil && (obj.Message != "" || len(obj.Code) > 0 || obj.Param != "") {
		code := strings.Trim(string(obj.Code), "\"")
		if code == "null" {
			code = ""
		}
		return &ProviderError{
			Message: boundedProviderMessage(obj.Message),
			Code:    boundedProviderMessage(code),
			Param:   boundedProviderMessage(obj.Param),
		}
	}
	return &ProviderError{Message: boundedProviderMessage(string(raw))}
}

func readErrorBody(r io.Reader) []byte {
	body, _ := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	return body
}

func newStreamScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLineBytes)
	return scanner
}

// IsStreamingUnsupported reports whether a provider error says the model or
// organisation cannot stream (the OpenAI cases upstream already handled).
func IsStreamingUnsupported(err error) bool {
	var pe *ProviderError
	if !errors.As(err, &pe) {
		return false
	}
	msg := strings.ToLower(pe.Message)
	return pe.Param == "stream" ||
		(pe.Code == "unsupported_value" && strings.Contains(msg, "stream")) ||
		strings.Contains(msg, "must be verified to stream")
}

// Both built-in providers report stream outcomes.
var (
	_ OutcomeStreamer = (*OllamaService)(nil)
	_ OutcomeStreamer = (*OpenAIService)(nil)
)
