package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaService handles Ollama API interactions
type OllamaService struct {
	baseURL string
	client  *http.Client
}

// NewOllamaService creates a new Ollama service
func NewOllamaService(baseURL string) *OllamaService {
	// Normalize base URL: remove trailing slash
	b := strings.TrimRight(baseURL, "/")
	return &OllamaService{
		baseURL: b,
		client:  &http.Client{Timeout: 60 * time.Minute},
	}
}

// Ollama tags response
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// GetModels retrieves available chat models from Ollama
func (s *OllamaService) GetModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	out := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out, nil
}

// Ollama chat API payloads
type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Options  map[string]any      `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// ChatCompletion performs a non-streaming chat completion against Ollama
func (s *OllamaService) ChatCompletion(ctx context.Context, model string, messages []ChatMessage, temperature float64) (*ChatResponse, error) {
	// Map to Ollama messages
	msgs := make([]ollamaChatMessage, 0, len(messages))
	for _, m := range messages {
		msgs = append(msgs, ollamaChatMessage(m))
	}
	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   false,
	}
	if temperature > 0 {
		reqBody.Options = map[string]any{"temperature": temperature}
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/chat", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}
	var oResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	// Map to generic ChatResponse
	cr := &ChatResponse{Model: oResp.Model}
	cr.Choices = []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{{
		Index: 0,
	}}
	cr.Choices[0].Message.Role = oResp.Message.Role
	cr.Choices[0].Message.Content = oResp.Message.Content
	return cr, nil
}

// ChatCompletionStream performs a streaming chat completion against Ollama
func (s *OllamaService) ChatCompletionStream(ctx context.Context, model string, messages []ChatMessage, temperature float64) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errorChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errorChan)

		msgs := make([]ollamaChatMessage, 0, len(messages))
		for _, m := range messages {
			msgs = append(msgs, ollamaChatMessage(m))
		}
		reqBody := ollamaChatRequest{Model: model, Messages: msgs, Stream: true}
		if temperature > 0 {
			reqBody.Options = map[string]any{"temperature": temperature}
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			errorChan <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}
		req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/chat", bytes.NewBuffer(data))
		if err != nil {
			errorChan <- fmt.Errorf("failed to create request: %w", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// The request body contains transcript and chat text. Never log it.
		resp, err := s.client.Do(req)
		if err != nil {
			errorChan <- fmt.Errorf("failed to make request: %w", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errorChan <- fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := scanner.Text()
			// Ollama streams JSON objects per line
			if strings.TrimSpace(line) == "" {
				continue
			}
			var chunk ollamaChatResponse
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}
			if chunk.Message.Content != "" {
				select {
				case contentChan <- chunk.Message.Content:
				case <-ctx.Done():
					return
				}
			}
			if chunk.Done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errorChan <- fmt.Errorf("error reading stream: %w", err)
		}
	}()

	return contentChan, errorChan
}

type ollamaStreamRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Format   json.RawMessage     `json:"format,omitempty"`
	Options  map[string]any      `json:"options,omitempty"`
}

type ollamaStreamChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	Error           string `json:"error"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// StreamWithOutcome streams a chat completion and reports how it ended.
// A stream that closes without a done=true chunk is reported as not completed;
// an {"error":...} line is reported as a ProviderError; done_reason is passed
// through so callers can detect an exhausted output budget ("length").
func (s *OllamaService) StreamWithOutcome(ctx context.Context, model string, messages []ChatMessage, opts GenerationOptions, onDelta func(string) error) (out StreamOutcome) {
	msgs := make([]ollamaChatMessage, 0, len(messages))
	for _, m := range messages {
		msgs = append(msgs, ollamaChatMessage(m))
	}
	options := map[string]any{}
	if opts.Temperature > 0 {
		options["temperature"] = opts.Temperature
	}
	if opts.MaxOutputTokens > 0 {
		options["num_predict"] = opts.MaxOutputTokens
	}
	if opts.ContextTokens > 0 {
		options["num_ctx"] = opts.ContextTokens
	}
	reqBody := ollamaStreamRequest{Model: model, Messages: msgs, Stream: true, Format: opts.JSONSchema}
	if len(options) > 0 {
		reqBody.Options = options
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		out.Err = fmt.Errorf("failed to marshal request: %w", err)
		return out
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		out.Err = fmt.Errorf("failed to create request: %w", err)
		return out
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			out.Err = ctxErr
			return out
		}
		out.Err = fmt.Errorf("%w: %v", ErrProviderUnreachable, err)
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out.Err = providerErrorFromBody(resp.StatusCode, readErrorBody(resp.Body))
		return out
	}

	scanner := newStreamScanner(resp.Body)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk ollamaStreamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			out.SkippedLines++
			continue
		}
		if chunk.Error != "" {
			out.Err = &ProviderError{Message: boundedProviderMessage(chunk.Error)}
			return out
		}
		if chunk.Message.Content != "" {
			if err := onDelta(chunk.Message.Content); err != nil {
				out.Err = fmt.Errorf("%w: %v", ErrDeltaRejected, err)
				return out
			}
		}
		if chunk.Done {
			out.Completed = true
			out.FinishReason = chunk.DoneReason
			out.PromptTokens = chunk.PromptEvalCount
			out.OutputTokens = chunk.EvalCount
			return out
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		out.Err = ctxErr
		return out
	}
	if err := scanner.Err(); err != nil {
		out.Err = fmt.Errorf("error reading stream: %w", err)
	}
	// Reaching EOF without done=true leaves Completed false: the stream is incomplete.
	return out
}

// ollamaShowRequest represents the request to show model info
type ollamaShowRequest struct {
	Name string `json:"name"`
}

// ollamaShowResponse represents the response from show model info
type ollamaShowResponse struct {
	ModelInfo map[string]interface{} `json:"model_info"`
	Details   struct {
		ContextLength int `json:"context_length"` // Some versions return this
	} `json:"details"`
	Parameters string `json:"parameters"`
}

// GetContextWindow returns the context window size for a given Ollama model
func (s *OllamaService) GetContextWindow(ctx context.Context, model string) (int, error) {
	// Default to 4096 if we can't determine
	defaultContext := 4096

	reqBody := ollamaShowRequest{
		Name: model,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return defaultContext, nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/show", bytes.NewBuffer(data))
	if err != nil {
		return defaultContext, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return defaultContext, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return defaultContext, nil
	}

	var showResp ollamaShowResponse
	if err := json.NewDecoder(resp.Body).Decode(&showResp); err != nil {
		return defaultContext, nil
	}

	// Try to find context length in details
	// Note: Ollama API response format varies.
	// Sometimes it's in model_info -> llama.context_length
	// Sometimes it's in parameters string "num_ctx 8192"

	// Check model_info
	if showResp.ModelInfo != nil {
		for k, v := range showResp.ModelInfo {
			if strings.Contains(k, "context_length") {
				if f, ok := v.(float64); ok {
					fmt.Printf("Debug: Found context length in model_info: %f\n", f)
					return int(f), nil
				}
			}
		}
	}

	// Parse parameters string
	if showResp.Parameters != "" {
		lines := strings.Split(showResp.Parameters, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "num_ctx") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					var ctxLen int
					if _, err := fmt.Sscanf(parts[1], "%d", &ctxLen); err == nil {
						fmt.Printf("Debug: Found context length in parameters: %d\n", ctxLen)
						return ctxLen, nil
					}
				}
			}
		}
	}

	fmt.Printf("Debug: Ollama context window for model %s: %d (default: %d)\n", model, defaultContext, 4096)
	return defaultContext, nil
}
