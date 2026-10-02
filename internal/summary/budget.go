package summary

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"scriberr/internal/llm"
)

// Defaults match scripts/windows/LocalMinutesSummary.Modelfile (num_ctx 8192,
// num_predict 2048). They are sent with every summary request so the runtime
// and this budget agree even if a model file differs.
const (
	DefaultContextTokens      = 8192
	DefaultMaxOutputTokens    = 2048
	DefaultSafetyMarginTokens = 256

	// Environment overrides for a differently provisioned model.
	EnvContextTokens   = "LOCAL_MINUTES_SUMMARY_CONTEXT_TOKENS"
	EnvMaxOutputTokens = "LOCAL_MINUTES_SUMMARY_MAX_OUTPUT_TOKENS"

	perMessageOverheadTokens = 16
	minContextTokens         = 2048
	minOutputTokens          = 256
)

// Limits bound one generation request.
type Limits struct {
	ContextTokens      int `json:"context_tokens"`
	MaxOutputTokens    int `json:"max_output_tokens"`
	SafetyMarginTokens int `json:"safety_margin_tokens"`
}

// DefaultLimits returns the qualified local configuration.
func DefaultLimits() Limits {
	return Limits{
		ContextTokens:      DefaultContextTokens,
		MaxOutputTokens:    DefaultMaxOutputTokens,
		SafetyMarginTokens: DefaultSafetyMarginTokens,
	}
}

// LimitsFromEnv applies optional overrides. Invalid values are rejected with
// an error so a misconfiguration is visible instead of silently ignored.
func LimitsFromEnv(getenv func(string) string) (Limits, error) {
	l := DefaultLimits()
	if v := strings.TrimSpace(getenv(EnvContextTokens)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < minContextTokens {
			return DefaultLimits(), fmt.Errorf("%s must be an integer >= %d", EnvContextTokens, minContextTokens)
		}
		l.ContextTokens = n
	}
	if v := strings.TrimSpace(getenv(EnvMaxOutputTokens)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < minOutputTokens {
			return DefaultLimits(), fmt.Errorf("%s must be an integer >= %d", EnvMaxOutputTokens, minOutputTokens)
		}
		l.MaxOutputTokens = n
	}
	if l.InputBudget() < 512 {
		return DefaultLimits(), fmt.Errorf("summary limits leave less than 512 input tokens")
	}
	return l, nil
}

// InputBudget is the space left for the prompt after reserving the output.
func (l Limits) InputBudget() int {
	return l.ContextTokens - l.MaxOutputTokens - l.SafetyMarginTokens
}

// GenerationOptions converts the limits into explicit provider options.
func (l Limits) GenerationOptions() llm.GenerationOptions {
	return llm.GenerationOptions{MaxOutputTokens: l.MaxOutputTokens, ContextTokens: l.ContextTokens}
}

// InputCheck is the result of comparing an estimated prompt with the budget.
type InputCheck struct {
	EstimatedTokens int  `json:"estimated_tokens"`
	BudgetTokens    int  `json:"budget_tokens"`
	Fits            bool `json:"fits"`
}

// CheckInput estimates the prompt size. Oversized input is rejected, never
// truncated: a cut transcript would silently drop the end of the meeting.
func (l Limits) CheckInput(messages []llm.ChatMessage) InputCheck {
	est := 0
	for _, m := range messages {
		est += perMessageOverheadTokens + EstimateTokens(m.Content)
	}
	return InputCheck{EstimatedTokens: est, BudgetTokens: l.InputBudget(), Fits: est <= l.InputBudget()}
}

// EstimateTokens deliberately over-estimates tokens for BPE models such as
// Qwen2.5: one token per digit, punctuation mark, newline and non-ASCII rune,
// and one per three ASCII letters of each word (rounded up). English prose is
// typically over-estimated by roughly 30-60%. Rejecting a borderline input is
// acceptable; overflowing the runtime context is not.
func EstimateTokens(s string) int {
	tokens, letters := 0, 0
	flush := func() {
		if letters > 0 {
			tokens += (letters + 2) / 3
			letters = 0
		}
	}
	for _, r := range s {
		switch {
		case r < utf8.RuneSelf && unicode.IsLetter(r):
			letters++
		case r == ' ' || r == '\t':
			flush()
		default:
			// Digits, punctuation, newlines and every non-ASCII rune.
			flush()
			tokens++
		}
	}
	flush()
	return tokens
}
