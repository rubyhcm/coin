package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// TelegramConfig holds bot credentials loaded from environment.
type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// LoadTelegramConfig reads Telegram credentials from environment.
func LoadTelegramConfig() TelegramConfig {
	return TelegramConfig{
		BotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		ChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
	}
}

// SendTelegramMessage sends a plain-text or Markdown message via Telegram Bot API.
func SendTelegramMessage(cfg TelegramConfig, text string) error {
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.BotToken)

	payload := map[string]interface{}{
		"chat_id":    cfg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API returned %d: %s", resp.StatusCode, string(raw))
	}

	return nil
}

// FormatTelegramMessage builds an HTML-formatted Telegram message from a prediction result.
func FormatTelegramMessage(r PredictionResult, currentPrice float64, interval string, t time.Time) string {
	directionEmoji := map[string]string{
		"up":       "📈",
		"down":     "📉",
		"sideways": "➡️",
	}
	tradeEmoji := map[string]string{
		"BUY":  "🟢",
		"SELL": "🔴",
		"WAIT": "⏸️",
	}

	dir := strings.ToLower(r.Direction)
	emoji := directionEmoji[dir]
	if emoji == "" {
		emoji = "❓"
	}

	ts := r.TradeSetup
	tradeIcon := tradeEmoji[ts.Type]
	if tradeIcon == "" {
		tradeIcon = "⏸️"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>⚡ XAU/USD — M%s Signal</b>\n", intervalLabel(interval)))
	sb.WriteString(fmt.Sprintf("<code>%s</code>\n\n", t.Format("02 Jan 2006  15:04 ICT")))

	sb.WriteString(fmt.Sprintf("💰 <b>Giá hiện tại:</b> <code>%.2f USD</code>\n", currentPrice))
	sb.WriteString(fmt.Sprintf("%s <b>Xu hướng:</b> %s  <b>|</b>  Tin cậy: <b>%.0f%%</b>\n", emoji, strings.ToUpper(r.Direction), r.Confidence*100))
	sb.WriteString(fmt.Sprintf("🎯 <b>Giá dự đoán:</b> <code>%.2f USD</code>\n", r.PredictedPrice))
	sb.WriteString(fmt.Sprintf("🔵 <b>Hỗ trợ:</b> <code>%.2f</code>  <b>|</b>  🔴 <b>Kháng cự:</b> <code>%.2f</code>\n", r.KeyLevels.Support, r.KeyLevels.Resistance))

	if ts.Type != "" {
		sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━\n")
		sb.WriteString(fmt.Sprintf("%s <b>Trade Setup: %s</b>\n", tradeIcon, ts.Type))
		if ts.Type != "WAIT" {
			sb.WriteString(fmt.Sprintf("  📌 Entry:   <code>%.2f</code>\n", ts.Entry))
			sb.WriteString(fmt.Sprintf("  🛑 SL:      <code>%.2f</code>\n", ts.StopLoss))
			sb.WriteString(fmt.Sprintf("  ✅ TP1:     <code>%.2f</code>\n", ts.TakeProfit1))
			sb.WriteString(fmt.Sprintf("  ✅ TP2:     <code>%.2f</code>\n", ts.TakeProfit2))
			if ts.RiskReward > 0 {
				sb.WriteString(fmt.Sprintf("  ⚖️  R:R:     <code>1 : %.2f</code>\n", ts.RiskReward))
			}
		}
	}

	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("📝 <i>%s</i>\n", r.Reasoning))
	sb.WriteString("\n<i>⚠️ AI analysis — not financial advice.</i>")

	return sb.String()
}

// intervalLabel converts "15min" → "15", "1h" → "60", etc. for display.
func intervalLabel(interval string) string {
	switch interval {
	case "1min":
		return "1"
	case "5min":
		return "5"
	case "15min":
		return "15"
	case "30min":
		return "30"
	case "45min":
		return "45"
	case "1h":
		return "60"
	case "2h":
		return "120"
	case "4h":
		return "H4"
	case "1day":
		return "D1"
	default:
		return interval
	}
}

// RunWatchLoop fetches data + predicts + sends Telegram message every `interval`.
// Only sends if confidence >= minConfidence. Runs until interrupted (Ctrl+C).
func RunWatchLoop(configs []Config, tgCfg TelegramConfig, histInterval string, histSize int, watchInterval time.Duration, minConfidence float64) {
	fmt.Printf("=== WATCH MODE: phân tích mỗi %v, gửi Telegram khi tin cậy ≥ %.0f%% ===\n", watchInterval, minConfidence*100)
	fmt.Printf("    Interval nến: %s | Số nến: %d\n\n", histInterval, histSize)

	tick := time.NewTicker(watchInterval)
	defer tick.Stop()

	// Run immediately on start, then every tick
	runOnce := func() {
		now := time.Now()
		fmt.Printf("[%s] Đang phân tích...\n", now.Format("15:04:05"))

		// Fetch historical data
		records, err := FetchHistoricalOHLC("XAU/USD", histInterval, histSize)
		if err != nil {
			fmt.Printf("  ❌ Lỗi lấy dữ liệu: %v\n", err)
			return
		}

		// Append live candle
		records, err = AppendLiveCandle(records, histInterval)
		if err != nil {
			fmt.Printf("  ⚠️  Không lấy được nến live: %v\n", err)
		}

		currentPrice := records[len(records)-1].Close

		// Predict with automatic fallback
		result, usedModel, err := PredictWithFallback(configs, records)
		if err != nil {
			fmt.Printf("  ❌ LLM lỗi: %v\n", err)
			return
		}
		fmt.Printf("  ✓ Model: %s\n", usedModel)

		// Print to terminal
		printPrediction(result)

		// Only send to Telegram if confidence >= minConfidence
		if result.Confidence < minConfidence {
			fmt.Printf("  ⏭️  Bỏ qua Telegram (tin cậy %.0f%% < %.0f%%)\n\n", result.Confidence*100, minConfidence*100)
			return
		}

		// Send to Telegram
		msg := FormatTelegramMessage(result, currentPrice, histInterval, now)
		if err := SendTelegramMessage(tgCfg, msg); err != nil {
			fmt.Printf("  ❌ Telegram lỗi: %v\n", err)
		} else {
			fmt.Printf("  ✅ Đã gửi Telegram lúc %s\n\n", now.Format("15:04:05"))
		}
	}

	runOnce()
	for range tick.C {
		runOnce()
	}
}
