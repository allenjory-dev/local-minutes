package summary

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"scriberr/internal/llm"
)

// Attempt status values. Only StatusCompleted can produce a saved summary.
const (
	StatusRunning    = "running"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusIncomplete = "incomplete"
	StatusCancelled  = "cancelled"
	StatusRejected   = "rejected"
	// StatusLegacy marks summaries saved before attempts were recorded.
	StatusLegacy = "legacy_unrecorded"
	// StatusNone is reported when a transcription has no saved summary.
	StatusNone = "none"
)

// Attempt reasons.
const (
	ReasonProviderError       = "provider_error"
	ReasonProviderUnreachable = "provider_unreachable"
	ReasonStreamReadError     = "stream_read_error"
	ReasonStreamEnded         = "stream_ended_without_completion"
	ReasonOutputLimit         = "output_limit_reached"
	ReasonUnexpectedFinish    = "unexpected_finish_reason"
	ReasonContextExceeded     = "context_limit_exceeded"
	ReasonEmptyOutput         = "empty_output"
	ReasonMalformedOutput     = "malformed_output"
	ReasonOutputTooLarge      = "output_too_large"
	ReasonCancelled           = "cancelled"
	ReasonTimedOut            = "timed_out"
	ReasonInputTooLarge       = "input_too_large"
	ReasonNoSegments          = "transcript_has_no_segments"
	ReasonInterrupted         = "interrupted"
	ReasonStorageError        = "storage_error"
)

// Classification is the application's verdict on one generation. Detail is
// written for the user and never contains model output.
type Classification struct {
	Status string
	Reason string
	Detail string
}

// Completed reports whether the generation may be saved.
func (c Classification) Completed() bool { return c.Status == StatusCompleted }

// ClassifyOutcome decides whether a stream produced a complete answer. It does
// not look at the meaning of the output; grounded drafts are parsed and
// validated afterwards.
func ClassifyOutcome(out llm.StreamOutcome, output string, limits Limits) Classification {
	if out.Err != nil {
		var pe *llm.ProviderError
		switch {
		case errors.Is(out.Err, context.Canceled) || errors.Is(out.Err, llm.ErrDeltaRejected):
			return Classification{StatusCancelled, ReasonCancelled, "Generation was cancelled before it finished."}
		case errors.Is(out.Err, context.DeadlineExceeded):
			return Classification{StatusIncomplete, ReasonTimedOut, "Generation exceeded the time limit before it finished."}
		case errors.As(out.Err, &pe):
			detail := "The model provider reported an error"
			if pe.StatusCode > 0 {
				detail += fmt.Sprintf(" (HTTP %d)", pe.StatusCode)
			}
			if pe.Message != "" {
				detail += ": " + pe.Message
			}
			return Classification{StatusFailed, ReasonProviderError, detail + "."}
		case errors.Is(out.Err, llm.ErrProviderUnreachable):
			return Classification{StatusFailed, ReasonProviderUnreachable, "The model provider could not be reached. Check that the local model server is running."}
		default:
			return Classification{StatusIncomplete, ReasonStreamReadError, "The connection to the model provider failed before generation finished."}
		}
	}
	if !out.Completed {
		return Classification{StatusIncomplete, ReasonStreamEnded, "The provider stream ended without a completion signal; the output may be cut off."}
	}
	switch strings.ToLower(out.FinishReason) {
	case "", "stop", "end_turn", "eos":
	case "length", "max_tokens":
		return Classification{StatusIncomplete, ReasonOutputLimit,
			fmt.Sprintf("The model reached the output limit (%d tokens) before finishing.", limits.MaxOutputTokens)}
	default:
		return Classification{StatusIncomplete, ReasonUnexpectedFinish,
			fmt.Sprintf("The provider ended generation with reason %q.", collapse(out.FinishReason))}
	}
	if limits.ContextTokens > 0 && out.PromptTokens > limits.ContextTokens-limits.MaxOutputTokens {
		return Classification{StatusIncomplete, ReasonContextExceeded,
			fmt.Sprintf("The provider reported %d prompt tokens, more than the planned input budget; the input may have been cut.", out.PromptTokens)}
	}
	if strings.TrimSpace(output) == "" {
		return Classification{StatusFailed, ReasonEmptyOutput, "The model returned no text."}
	}
	return Classification{Status: StatusCompleted}
}

// HTTPStatus maps a failed classification to an HTTP status code.
func (c Classification) HTTPStatus() int {
	switch c.Reason {
	case "":
		return 201
	case ReasonInputTooLarge:
		return 413
	case ReasonNoSegments:
		return 422
	case ReasonCancelled:
		return 499
	case ReasonTimedOut:
		return 504
	case ReasonStorageError:
		return 500
	default:
		return 502
	}
}
