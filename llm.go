package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMClient wraps an OpenAI-compatible chat completion endpoint.
type LLMClient struct {
	cfg     Config
	httpCli *http.Client
}

// NewLLMClient creates a client with the given config.
func NewLLMClient(cfg Config) *LLMClient {
	return &LLMClient{
		cfg: cfg,
		httpCli: &http.Client{Timeout: 60 * time.Second},
	}
}

// chatRequest is the OpenAI-compatible chat completion request body.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is a minimal subset of the OpenAI chat completion response.
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Predict sends historical price data to the LLM and returns a structured prediction.
func (c *LLMClient) Predict(history []PriceRecord) (PredictionResult, error) {
	prompt := buildPrompt(history)

	reqBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{
				Role: "system",
				Content: "You are a financial market analyst specializing in gold (XAU/USD) price analysis. " +
					"Analyze the provided historical price data and predict the likely direction and price " +
					"for the next period. Always respond with valid JSON only, no markdown formatting.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3, // low temperature for more consistent analysis
		MaxTokens:   4000, // reasoning models need headroom for thinking + output
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return PredictionResult{}, fmt.Errorf("marshaling request: %w", err)
	}

	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"

	// Retry with backoff on rate-limit (429) or server (5xx) errors.
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Second // 2s, 4s, 8s
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			return PredictionResult{}, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

		resp, err := c.httpCli.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("API request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading response: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
			continue // retry
		}

		if resp.StatusCode != http.StatusOK {
			return PredictionResult{}, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return PredictionResult{}, fmt.Errorf("parsing API response: %w", err)
		}

		if len(chatResp.Choices) == 0 {
			return PredictionResult{}, fmt.Errorf("no choices in API response")
		}

		content := chatResp.Choices[0].Message.Content

		// The LLM may wrap JSON in code fences; strip them.
		content = stripCodeFences(content)

		// Empty response — retry with a shorter prompt (free models have token limits)
		if strings.TrimSpace(content) == "" {
			lastErr = fmt.Errorf("empty response from model")
			continue
		}

		var result PredictionResult
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return PredictionResult{}, fmt.Errorf("parsing prediction JSON (raw: %s): %w", content, err)
		}

		return result, nil
	}

	return PredictionResult{}, fmt.Errorf("API request failed after %d retries: %w", maxRetries, lastErr)
}

// buildPrompt constructs the user prompt with historical price context.
// Only the most recent 30 candles are sent to keep the prompt within free-tier token limits.
func buildPrompt(history []PriceRecord) string {
	const maxCandles = 30
	if len(history) > maxCandles {
		history = history[len(history)-maxCandles:]
	}

	var sb strings.Builder
	sb.WriteString("Analyze the following GOLD/USD historical price data and predict the next period's price movement.\n\n")
	sb.WriteString("Historical data (most recent last):\n\n")
	sb.WriteString("| Date                | Open    | High    | Low     | Close   |\n")
	sb.WriteString("|---------------------|---------|---------|---------|---------|\n")
	for _, r := range history {
		sb.WriteString(fmt.Sprintf("| %-19s | %.2f | %.2f | %.2f | %.2f |\n", r.Date, r.Open, r.High, r.Low, r.Close))
	}
	sb.WriteString("\n")
	sb.WriteString("Respond with ONLY a JSON object, no markdown, no extra text:\n")
	sb.WriteString(`{"direction":"up"|"down"|"sideways","confidence":<0.0-1.0>,"predicted_price":<number>,"reasoning":"<brief>","key_levels":{"support":<number>,"resistance":<number>}}`)

	return sb.String()
}

// stripCodeFences removes ```json ... ``` wrappers if the LLM added them.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove first line (```json or ```)
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove trailing ```
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	return s
}
