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
		httpCli: &http.Client{Timeout: 120 * time.Second},
	}
}

// PredictWithFallback tries each provider in order and returns the first success.
// If a provider fails (rate limit, quota, error), it automatically switches to the next.
func PredictWithFallback(configs []Config, history []PriceRecord) (PredictionResult, string, error) {
	var lastErr error
	for _, cfg := range configs {
		client := NewLLMClient(cfg)
		result, err := client.Predict(history)
		if err == nil {
			return result, cfg.Model, nil
		}
		fmt.Printf("⚠  Provider %s failed: %v — trying next...\n", cfg.Model, err)
		lastErr = err
	}
	return PredictionResult{}, "", fmt.Errorf("all providers failed, last error: %w", lastErr)
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
		Message struct {
			Content  *string `json:"content"`   // nil for reasoning-only models
			Reasoning string  `json:"reasoning"` // populated by reasoning models (e.g. Nemotron)
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
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
				Content: "You are an expert gold (XAU/USD) trading analyst. " +
					"Analyze the provided OHLC candlestick data and return a structured trade recommendation. " +
					"Identify trend direction, key support/resistance levels, and provide a concrete trade setup " +
					"with entry price, stop loss, and two take profit targets based on technical analysis. " +
					"Always respond with valid JSON only, no markdown formatting.",
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

		content := ""
		if chatResp.Choices[0].Message.Content != nil {
			content = *chatResp.Choices[0].Message.Content
		}

		// Reasoning models (e.g. Nemotron) put their output inside the reasoning
		// field and leave content null. Extract the JSON block from reasoning.
		if strings.TrimSpace(content) == "" && chatResp.Choices[0].Message.Reasoning != "" {
			content = extractJSONFromReasoning(chatResp.Choices[0].Message.Reasoning)
		}

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

		// Fallback: if model didn't return trade_setup, derive it from key_levels
		if result.TradeSetup.Type == "" && result.KeyLevels.Support > 0 && result.KeyLevels.Resistance > 0 {
			result.TradeSetup = deriveTradeSetup(result.Direction, result.PredictedPrice,
				result.KeyLevels.Support, result.KeyLevels.Resistance)
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

	// Current price = last close
	lastClose := history[len(history)-1].Close

	var sb strings.Builder
	sb.WriteString("Analyze the following XAU/USD candlestick data and provide a trade recommendation.\n\n")
	sb.WriteString("OHLC data (most recent last):\n\n")
	sb.WriteString("| Date                | Open    | High    | Low     | Close   |\n")
	sb.WriteString("|---------------------|---------|---------|---------|---------|\n")
	for _, r := range history {
		sb.WriteString(fmt.Sprintf("| %-19s | %.2f | %.2f | %.2f | %.2f |\n",
			r.Date, r.Open, r.High, r.Low, r.Close))
	}
	sb.WriteString(fmt.Sprintf("\nCurrent price: %.2f USD\n\n", lastClose))
	sb.WriteString("Respond with ONLY this JSON, no markdown, no extra text:\n")
	sb.WriteString(`{`)
	sb.WriteString(`"direction":"up"|"down"|"sideways",`)
	sb.WriteString(`"confidence":<0.0-1.0>,`)
	sb.WriteString(`"predicted_price":<next period close>,`)
	sb.WriteString(`"reasoning":"<2-3 sentences citing trend, S/R, momentum>",`)
	sb.WriteString(`"key_levels":{"support":<number>,"resistance":<number>},`)
	sb.WriteString(`"trade_setup":{`)
	sb.WriteString(`"type":"BUY"|"SELL"|"WAIT",`)
	sb.WriteString(`"entry":<best entry price>,`)
	sb.WriteString(`"stop_loss":<stop loss price beyond nearest S/R>,`)
	sb.WriteString(`"take_profit_1":<conservative TP at nearest S/R>,`)
	sb.WriteString(`"take_profit_2":<extended TP at next S/R>,`)
	sb.WriteString(`"risk_reward":<R:R ratio to TP1 as decimal e.g. 2.0>`)
	sb.WriteString(`}}`)

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

// deriveTradeSetup calculates a basic trade setup from direction and key levels
// when the model did not return a trade_setup block.
func deriveTradeSetup(direction string, predictedPrice, support, resistance float64) struct {
	Type        string  `json:"type"`
	Entry       float64 `json:"entry"`
	StopLoss    float64 `json:"stop_loss"`
	TakeProfit1 float64 `json:"take_profit_1"`
	TakeProfit2 float64 `json:"take_profit_2"`
	RiskReward  float64 `json:"risk_reward"`
} {
	type ts = struct {
		Type        string  `json:"type"`
		Entry       float64 `json:"entry"`
		StopLoss    float64 `json:"stop_loss"`
		TakeProfit1 float64 `json:"take_profit_1"`
		TakeProfit2 float64 `json:"take_profit_2"`
		RiskReward  float64 `json:"risk_reward"`
	}
	spread := 0.30 // typical XAU/USD spread buffer

	switch direction {
	case "up":
		entry := support + spread
		sl := support - (resistance-support)*0.3
		tp1 := resistance - spread
		tp2 := resistance + (resistance-support)*0.5
		rr := 0.0
		if entry-sl > 0 {
			rr = (tp1 - entry) / (entry - sl)
		}
		return ts{"BUY", entry, sl, tp1, tp2, rr}
	case "down":
		entry := resistance - spread
		sl := resistance + (resistance-support)*0.3
		tp1 := support + spread
		tp2 := support - (resistance-support)*0.5
		rr := 0.0
		if sl-entry > 0 {
			rr = (entry - tp1) / (sl - entry)
		}
		return ts{"SELL", entry, sl, tp1, tp2, rr}
	default:
		return ts{Type: "WAIT"}
	}
}

// extractJSONFromReasoning finds the last complete JSON object {...} in a reasoning
// block. Reasoning models often finish their thinking with the actual JSON answer.
func extractJSONFromReasoning(reasoning string) string {
	// Find the last '{' that starts a complete JSON object
	lastOpen := strings.LastIndex(reasoning, "{")
	if lastOpen == -1 {
		return ""
	}

	// Find matching closing brace by counting depth
	depth := 0
	for i := lastOpen; i < len(reasoning); i++ {
		switch reasoning[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := strings.TrimSpace(reasoning[lastOpen : i+1])
				// Quick sanity check: must have at least "direction" key
				if strings.Contains(candidate, "direction") {
					return candidate
				}
				// Not the right block — keep searching backwards
				lastOpen = strings.LastIndex(reasoning[:lastOpen], "{")
				if lastOpen == -1 {
					return ""
				}
				i = lastOpen - 1
				depth = 0
			}
		}
	}
	return ""
}
