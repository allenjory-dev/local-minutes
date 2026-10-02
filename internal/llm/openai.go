package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// OpenAIService handles OpenAI API interactions
type OpenAIService struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIService creates a new OpenAI service
func NewOpenAIService(apiKey string, baseURL *string) *OpenAIService {
	url := "https://api.openai.com/v1"
	if baseURL != nil && *baseURL != "" {
		url = *baseURL
	}
	return &OpenAIService{
		apiKey:  apiKey,
		baseURL: url,
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

// ChatMessage represents a chat message for OpenAI API
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents the OpenAI chat completion request
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse represents the OpenAI chat completion response
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// ChatStreamResponse represents a streaming response chunk
type ChatStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ModelsResponse represents the OpenAI models list response
type ModelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// GetModels retrieves available chat models from OpenAI
func (s *OpenAIService) GetModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
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

	var modelsResp ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	useDefault := s.baseURL == "https://api.openai.com/v1"
	var chatModels []string
	for _, model := range modelsResp.Data {
		if useDefault {
			// Filter for chat models (GPT models) if default OpenAI baseURL
			if strings.Contains(model.ID, "gpt") {
				chatModels = append(chatModels, model.ID)
			}
		} else {
			// If custom baseURL → return all models
			chatModels = append(chatModels, model.ID)
		}
	}

	return chatModels, nil
}

// ChatCompletion performs a non-streaming chat completion
func (s *OpenAIService) ChatCompletion(ctx context.Context, model string, messages []ChatMessage, temperature float64) (*ChatResponse, error) {
	// Build request without temperature to use model defaults.
	reqBody := ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	// Only set temperature if caller provided a non-zero value.
	if temperature != 0 {
		reqBody.Temperature = temperature
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[openai] chat completion request model=%s messages=%d stream=%v", model, len(messages), false)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Provider error bodies can echo request content; log size only.
		log.Printf("[openai] chat completion error status=%d body_bytes=%d", resp.StatusCode, len(body))
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("[openai] chat completion ok model=%s choices=%d", model, len(chatResp.Choices))
	return &chatResp, nil
}

// ChatCompletionStream performs a streaming chat completion
func (s *OpenAIService) ChatCompletionStream(ctx context.Context, model string, messages []ChatMessage, temperature float64) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errorChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errorChan)

		// Build request without temperature to use model defaults.
		reqBody := ChatRequest{
			Model:    model,
			Messages: messages,
			Stream:   true,
		}
		// Only set temperature if caller provided a non-zero value.
		if temperature != 0 {
			reqBody.Temperature = temperature
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			errorChan <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			errorChan <- fmt.Errorf("failed to create request: %w", err)
			return
		}

		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		log.Printf("[openai] chat stream request model=%s messages=%d stream=%v", model, len(messages), true)
		resp, err := s.client.Do(req)
		if err != nil {
			errorChan <- fmt.Errorf("failed to make request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			// Provider error bodies can echo request content; log size only.
			log.Printf("[openai] chat stream error status=%d body_bytes=%d", resp.StatusCode, len(body))
			errorChan <- fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		loggedFirst := false
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and comments
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			// Remove "data: " prefix
			data := strings.TrimPrefix(line, "data: ")

			// Check for end of stream
			if data == "[DONE]" {
				log.Printf("[openai] chat stream done model=%s", model)
				return
			}

			// Parse the JSON chunk
			var chunk ChatStreamResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// Skip invalid JSON chunks
				continue
			}

			// Extract content from the chunk
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case contentChan <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
				if !loggedFirst {
					loggedFirst = true
					log.Printf("[openai] chat stream first content model=%s", model)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errorChan <- fmt.Errorf("error reading stream: %w", err)
		}
	}()

	return contentChan, errorChan
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// StreamWithOutcome streams a chat completion and reports how it ended.
// Completion requires an explicit "data: [DONE]" or a finish_reason; a stream
// that closes without either is reported as not completed. ContextTokens and
// JSONSchema are not sent because OpenAI-compatible servers differ in support.
func (s *OpenAIService) StreamWithOutcome(ctx context.Context, model string, messages []ChatMessage, opts GenerationOptions, onDelta func(string) error) (out StreamOutcome) {
	reqBody := ChatRequest{Model: model, Messages: messages, Stream: true}
	if opts.Temperature != 0 {
		reqBody.Temperature = opts.Temperature
	}
	if opts.MaxOutputTokens > 0 {
		reqBody.MaxTokens = opts.MaxOutputTokens
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		out.Err = fmt.Errorf("failed to marshal request: %w", err)
		return out
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		out.Err = fmt.Errorf("failed to create request: %w", err)
		return out
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

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

	sawFinish := false
	scanner := newStreamScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // blank separators, comments and event names
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			out.Completed = true
			return out
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			out.SkippedLines++
			continue
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			out.Err = providerErrorFromField(chunk.Error)
			return out
		}
		if chunk.Usage != nil {
			out.PromptTokens = chunk.Usage.PromptTokens
			out.OutputTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.Delta.Content != "" {
			if err := onDelta(choice.Delta.Content); err != nil {
				out.Err = fmt.Errorf("%w: %w", ErrDeltaRejected, err)
				return out
			}
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			out.FinishReason = *choice.FinishReason
			sawFinish = true
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		out.Err = ctxErr
		return out
	}
	if err := scanner.Err(); err != nil {
		out.Err = fmt.Errorf("error reading stream: %w", err)
		return out
	}
	// A finish_reason is an explicit end-of-generation signal even if the
	// server closed the connection before sending [DONE].
	out.Completed = sawFinish
	return out
}

// ValidateAPIKey validates the provided API key by making a test request
func (s *OpenAIService) ValidateAPIKey(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid API key")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %d", resp.StatusCode)
	}

	return nil
}

// GetContextWindow returns the context window size for a given OpenAI model
func (s *OpenAIService) GetContextWindow(ctx context.Context, model string) (int, error) {
	// Known context windows for OpenAI models
	// As of late 2024/early 2025
	switch {
	case strings.HasPrefix(model, "gpt-4-turbo"), strings.HasPrefix(model, "gpt-4o"):
		return 128000, nil
	case strings.HasPrefix(model, "gpt-4-32k"):
		return 32768, nil
	case strings.HasPrefix(model, "gpt-4"):
		return 8192, nil
	case strings.HasPrefix(model, "gpt-3.5-turbo-16k"):
		return 16385, nil
	case strings.HasPrefix(model, "gpt-3.5-turbo"):
		return 16385, nil // Most recent gpt-3.5-turbo is 16k
	default:
		// Default fallback
		return 4096, nil
	}
}
