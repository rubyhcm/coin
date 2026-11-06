package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// loadDotEnv reads key=value pairs from a .env file and sets them as environment
// variables. Existing env vars are NOT overwritten (shell always wins).
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip optional surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" { // don't overwrite shell env
			os.Setenv(key, val)
		}
	}
}

// Config holds the LLM API credentials and settings loaded from environment.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

// LoadConfig reads LLM settings from environment variables with sensible defaults.
func LoadConfig() Config {
	return Config{
		APIKey:  getEnv("LLM_API_KEY", ""),
		BaseURL: getEnv("LLM_BASE_URL", "https://api.openai.com/v1"),
		Model:   getEnv("LLM_MODEL", "gpt-4o-mini"),
	}
}

// providerPresets maps provider name → (baseURL, model, envKeyName).
// envKeyName is the env var that holds the API key for that provider.
var providerPresets = map[string][3]string{
	"openrouter": {
		"https://openrouter.ai/api/v1",
		"nvidia/nemotron-3-ultra-550b-a55b:free",
		"OPENROUTER_API_KEY",
	},
	"groq": {
		"https://api.groq.com/openai/v1",
		"qwen/qwen3.8-27b",
		"GROQ_API_KEY",
	},
	"gemini": {
		"https://generativelanguage.googleapis.com/v1beta/openai/",
		"gemini-2.5-flash",
		"GEMINI_API_KEY",
	},
}

// applyProvider overrides cfg with preset values for the given provider name.
// The provider-specific API key env var takes precedence over LLM_API_KEY.
func applyProvider(cfg Config, provider string) Config {
	preset, ok := providerPresets[strings.ToLower(provider)]
	if !ok {
		log.Fatalf("Unknown provider %q. Available: openrouter, groq, gemini", provider)
	}
	cfg.BaseURL = preset[0]
	cfg.Model = preset[1]
	// Use provider-specific key if set, otherwise fall back to LLM_API_KEY
	if key := os.Getenv(preset[2]); key != "" {
		cfg.APIKey = key
	}
	return cfg
}

// providerOrder defines the default fallback sequence when -provider is not specified.
// Providers without a key configured are skipped automatically.
var providerOrder = []string{"openrouter", "groq", "gemini"}

// buildFallbackChain returns an ordered list of Configs to try.
// If -provider is set, that provider goes first, then the rest follow.
// Providers with no API key are excluded.
func buildFallbackChain(defaultCfg Config, firstProvider string) []Config {
	// Build a base key from default cfg (LLM_API_KEY) for the default provider
	var chain []Config

	// Determine the ordered list: firstProvider first, then the rest
	order := providerOrder
	if firstProvider != "" {
		rest := make([]string, 0, len(providerOrder))
		for _, p := range providerOrder {
			if strings.ToLower(p) != strings.ToLower(firstProvider) {
				rest = append(rest, p)
			}
		}
		order = append([]string{strings.ToLower(firstProvider)}, rest...)
	}

	for _, name := range order {
		preset := providerPresets[name]
		key := os.Getenv(preset[2])
		if key == "" {
			key = defaultCfg.APIKey // fallback to LLM_API_KEY
		}
		if key == "" {
			continue // no key at all — skip this provider
		}
		chain = append(chain, Config{
			APIKey:  key,
			BaseURL: preset[0],
			Model:   preset[1],
		})
	}

	// If chain is empty or user uses custom LLM_BASE_URL, prepend the default cfg
	if len(chain) == 0 {
		chain = append(chain, defaultCfg)
	}

	return chain
}

// PriceRecord represents a single historical OHLC price point.
type PriceRecord struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume,omitempty"`
}

// PredictionResult is the structured response we expect from the LLM.
type PredictionResult struct {
	Direction      string  `json:"direction"`       // "up", "down", or "sideways"
	Confidence     float64 `json:"confidence"`      // 0.0 - 1.0
	PredictedPrice float64 `json:"predicted_price"` // estimated next-period close price
	Reasoning      string  `json:"reasoning"`       // explanation of the analysis
	KeyLevels      struct {
		Support    float64 `json:"support"`
		Resistance float64 `json:"resistance"`
	} `json:"key_levels"`
	TradeSetup struct {
		Type       string  `json:"type"`        // "BUY", "SELL", or "WAIT"
		Entry      float64 `json:"entry"`       // suggested entry price
		StopLoss   float64 `json:"stop_loss"`   // stop loss price
		TakeProfit1 float64 `json:"take_profit_1"` // first TP (conservative)
		TakeProfit2 float64 `json:"take_profit_2"` // second TP (extended)
		RiskReward float64 `json:"risk_reward"` // R:R ratio to TP1
	} `json:"trade_setup"`
}

func main() {
	// Auto-load .env from current directory (shell env vars take precedence)
	loadDotEnv(".env")

	// CLI flags
	historicalFile := flag.String("data", "", "Path to JSON file with historical price data (array of {date,open,high,low,close})")
	interactive := flag.Bool("i", false, "Interactive mode: paste historical data via stdin")
	live := flag.Bool("live", false, "Fetch real-time gold price and predict based on live data")
	liveSnapshots := flag.Int("live-snapshots", 10, "Number of OHLC candles to collect (only with -live)")
	liveCandle := flag.Duration("live-candle", 15*time.Minute, "Duration of each OHLC candle, e.g. 15m (only with -live)")
	liveInterval := flag.Duration("live-interval", 30*time.Second, "Poll interval within each candle (only with -live)")
	chart := flag.Bool("chart", false, "Generate an HTML candlestick chart (MT5-style) and open in browser")
	chartOut := flag.String("chart-out", "chart.html", "Output path for the HTML chart (only with -chart)")
	historyMode := flag.Bool("history", false, "Fetch historical XAU/USD OHLC candles from Twelve Data (requires TWELVEDATA_API_KEY)")
	historyInterval := flag.String("history-interval", "15min", "Candle interval for historical data (e.g. 15min, 30min, 1h, 1day)")
	historySize := flag.Int("history-size", 100, "Number of historical candles to fetch (only with -history)")
	watchMode := flag.Bool("watch", false, "Watch mode: analyse repeatedly and send results to Telegram (requires TELEGRAM_BOT_TOKEN + TELEGRAM_CHAT_ID)")
	watchInterval := flag.Duration("watch-interval", 3*time.Minute, "How often to analyse and send in watch mode (default 3m)")
	minConfidence := flag.Float64("min-confidence", 0.75, "Minimum confidence threshold to send Telegram alert (0.0-1.0, default 0.75)")
	provider := flag.String("provider", "", "LLM provider override: openrouter | groq | gemini (overrides LLM_BASE_URL and LLM_MODEL)")
	flag.Parse()

	cfg := LoadConfig()
	// Build fallback chain: -provider goes first, then the rest in order.
	// Providers with no key are skipped automatically.
	fallbackChain := buildFallbackChain(cfg, *provider)
	if len(fallbackChain) == 0 {
		log.Fatal("No LLM provider configured. Set at least one of: LLM_API_KEY, OPENROUTER_API_KEY, GROQ_API_KEY, GEMINI_API_KEY")
	}
	if *provider != "" {
		fmt.Printf("Provider: %s (with fallback to others if needed)\n", strings.ToUpper(*provider))
	}

	// Watch mode: run loop, send to Telegram, exit early
	if *watchMode {
		tgCfg := LoadTelegramConfig()
		if tgCfg.BotToken == "" || tgCfg.ChatID == "" {
			log.Fatal("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID must be set for -watch mode.")
		}
		RunWatchLoop(fallbackChain, tgCfg, *historyInterval, *historySize, *watchInterval, *minConfidence)
		return
	}

	// Load historical data
	var history []PriceRecord
	var err error

	switch {
	case *historyMode:
		// Historical mode: fetch OHLC candles from Twelve Data API
		fmt.Printf("=== HISTORY MODE: Fetching %d historical XAU/USD candles (%s interval) ===\n",
			*historySize, *historyInterval)
		histRecords, err := FetchHistoricalOHLC("XAU/USD", *historyInterval, *historySize)
		if err != nil {
			log.Fatalf("Failed to fetch historical data: %v", err)
		}
		// Append the currently forming candle from real-time price
		histRecords, err = AppendLiveCandle(histRecords, *historyInterval)
		if err != nil {
			fmt.Printf("Warning: could not fetch live candle: %v\n", err)
		}
		history = histRecords
	case *live:
		// Real-time mode: fetch live gold price from gold-api.com (free, no API key)
		fmt.Printf("=== LIVE MODE: %d OHLC candles × %v (poll every %v) ===\n",
			*liveSnapshots, *liveCandle, *liveInterval)
		history, err = FetchLiveOHLCHistory(*liveSnapshots, *liveCandle, *liveInterval)
	case *historicalFile != "":
		history, err = loadPriceData(*historicalFile)
	case *interactive:
		history, err = readPriceDataStdin()
	default:
		// Demo mode: generate sample recent gold price data
		fmt.Println("No data file provided. Using demo data (sample gold prices).")
		history = generateDemoData()
	}
	if err != nil {
		log.Fatalf("Failed to load price data: %v", err)
	}

	if len(history) == 0 {
		log.Fatal("No historical data available.")
	}

	fmt.Printf("Loaded %d price records.\n", len(history))
	fmt.Printf("Last close: %.2f USD on %s\n", history[len(history)-1].Close, history[len(history)-1].Date)

	// Generate HTML candlestick chart if requested
	if *chart {
		if err := generateChartHTML(ChartOptions{
			Title:    "XAU/USD Gold Price",
			Records:  history,
			OutPath:  *chartOut,
			AutoOpen: true,
		}); err != nil {
			fmt.Printf("Warning: chart generation failed: %v\n", err)
		}
	}

	fmt.Println("Analyzing with LLM...")
	fmt.Println()

	result, usedModel, err := PredictWithFallback(fallbackChain, history)
	if err != nil {
		log.Fatalf("Prediction failed: %v", err)
	}
	fmt.Printf("✓ Model used: %s\n\n", usedModel)

	printPrediction(result)
}

// printPrediction formats the prediction result for display.
func printPrediction(r PredictionResult) {
	fmt.Println("════════════════════════════════════════════")
	fmt.Println("       GOLD/USD PRICE PREDICTION")
	fmt.Println("════════════════════════════════════════════")
	fmt.Printf("Direction        : %s\n", strings.ToUpper(r.Direction))
	fmt.Printf("Confidence       : %.0f%%\n", r.Confidence*100)
	fmt.Printf("Predicted Price  : %.2f USD\n", r.PredictedPrice)
	fmt.Printf("Support Level    : %.2f USD\n", r.KeyLevels.Support)
	fmt.Printf("Resistance Level : %.2f USD\n", r.KeyLevels.Resistance)

	ts := r.TradeSetup
	if ts.Type != "" && ts.Type != "WAIT" {
		fmt.Println("────────────────────────────────────────────")
		fmt.Printf("Trade Setup      : %s\n", ts.Type)
		fmt.Printf("Entry            : %.2f USD\n", ts.Entry)
		fmt.Printf("Stop Loss        : %.2f USD\n", ts.StopLoss)
		fmt.Printf("Take Profit 1    : %.2f USD\n", ts.TakeProfit1)
		fmt.Printf("Take Profit 2    : %.2f USD\n", ts.TakeProfit2)
		if ts.RiskReward > 0 {
			fmt.Printf("Risk/Reward      : 1 : %.2f\n", ts.RiskReward)
		}
	} else if ts.Type == "WAIT" {
		fmt.Println("────────────────────────────────────────────")
		fmt.Println("Trade Setup      : WAIT — không vào lệnh lúc này")
	}

	fmt.Println("────────────────────────────────────────────")
	fmt.Println("Analysis:")
	fmt.Println(r.Reasoning)
	fmt.Println("════════════════════════════════════════════")
	fmt.Println("⚠️  DISCLAIMER: This is AI-generated analysis, not financial advice.")
}

// loadPriceData reads a JSON array of PriceRecord from a file.
func loadPriceData(path string) ([]PriceRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var records []PriceRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parsing JSON (expected array of {date,open,high,low,close}): %w", err)
	}
	return records, nil
}

// readPriceDataStdin reads JSON price data pasted by the user.
func readPriceDataStdin() ([]PriceRecord, error) {
	fmt.Println("Paste historical price data as JSON array, then press Ctrl+D:")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
	}

	var records []PriceRecord
	if err := json.Unmarshal([]byte(sb.String()), &records); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return records, nil
}

// generateDemoData creates a sample series of gold prices for testing.
func generateDemoData() []PriceRecord {
	// Approximate recent gold prices (for demo purposes only)
	basePrices := []float64{2350, 2360, 2372, 2365, 2380, 2390, 2401, 2395, 2410, 2425, 2418, 2430}
	records := make([]PriceRecord, len(basePrices))
	now := time.Now()
	for i, close := range basePrices {
		date := now.AddDate(0, 0, -(len(basePrices) - i - 1))
		open := close - 3
		records[i] = PriceRecord{
			Date:   date.Format("2006-01-02"),
			Open:   open,
			High:   close + 5,
			Low:    open - 4,
			Close:  close,
			Volume: 100000,
		}
	}
	return records
}

// getEnv reads an environment variable with a fallback default.
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
