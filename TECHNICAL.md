# CoinPredict — Tài liệu kỹ thuật

## Tổng quan

CoinPredict là ứng dụng CLI viết bằng Go, lấy dữ liệu giá vàng XAU/USD dưới dạng nến OHLC, vẽ biểu đồ nến kiểu MT5, gọi LLM (tương thích OpenAI) để dự đoán xu hướng và đề xuất trade setup (entry / SL / TP), và gửi kết quả lên Telegram theo định kỳ.

Không có Go module dependency ngoài standard library.

---

## Cấu trúc file

```
main.go       — CLI, flags, routing chế độ, orchestration
price.go      — Lấy dữ liệu giá (gold-api.com, Twelve Data)
llm.go        — Gọi LLM API, build prompt, parse kết quả
chart.go      — Tạo file HTML biểu đồ nến MT5-style
telegram.go   — Gửi phân tích qua Telegram Bot API
```

---

## Luồng xử lý

```
main()
 ├── loadDotEnv(".env")
 ├── parse flags
 ├── LoadConfig()               # LLM_API_KEY, LLM_BASE_URL, LLM_MODEL
 │
 ├── [-watch] ──────────────────────────────────────────────────────────┐
 │    └── RunWatchLoop()        # lặp vô hạn theo watchInterval         │
 │         ├── FetchHistoricalOHLC() + AppendLiveCandle()               │
 │         ├── LLMClient.Predict()                                      │
 │         ├── printPrediction()                                        │
 │         └── [confidence ≥ minConfidence] → SendTelegramMessage()     │
 │                                                                      │
 ├── [switch chế độ 1 lần] ◄───────────────────────────────────────────┘
 │    ├── -history  → FetchHistoricalOHLC() + AppendLiveCandle()
 │    ├── -live     → FetchLiveOHLCHistory()
 │    ├── -data     → loadPriceData()
 │    ├── -i        → readPriceDataStdin()
 │    └── (default) → generateDemoData()
 │
 ├── [-chart] → generateChartHTML()     # tạo HTML, mở browser
 │
 └── LLMClient.Predict() → printPrediction()
```

---

## Các file và hàm

### `main.go`

| Hàm / Type | Mô tả |
|---|---|
| `loadDotEnv(path)` | Đọc `KEY=VALUE` từ `.env`, set env vars. Shell env được ưu tiên (không overwrite). Bỏ qua nếu file không tồn tại. |
| `Config` | Struct: `APIKey`, `BaseURL`, `Model` cho LLM. |
| `PriceRecord` | Struct OHLC: `Date`, `Open`, `High`, `Low`, `Close`, `Volume`. Date kết thúc bằng `⟳` nếu là nến đang hình thành. |
| `PredictionResult` | Struct kết quả LLM. Xem chi tiết bên dưới. |
| `main()` | Parse flags, load data, gọi chart và LLM. |
| `printPrediction(r)` | In kết quả phân tích + trade setup ra stdout. |
| `loadPriceData(path)` | Đọc JSON file → `[]PriceRecord`. |
| `readPriceDataStdin()` | Đọc JSON từ stdin (chế độ `-i`). |
| `generateDemoData()` | 12 bản ghi mẫu cho demo. |
| `getEnv(key, fallback)` | Đọc env var với fallback. |

**`PredictionResult`:**
```go
type PredictionResult struct {
    Direction      string  // "up" | "down" | "sideways"
    Confidence     float64 // 0.0 – 1.0
    PredictedPrice float64
    Reasoning      string
    KeyLevels      struct {
        Support    float64
        Resistance float64
    }
    TradeSetup struct {
        Type        string  // "BUY" | "SELL" | "WAIT"
        Entry       float64
        StopLoss    float64
        TakeProfit1 float64 // conservative
        TakeProfit2 float64 // extended
        RiskReward  float64 // R:R ratio to TP1
    }
}
```

---

### `price.go`

#### Nguồn dữ liệu

| Nguồn | API | Auth | Giới hạn |
|---|---|---|---|
| gold-api.com | `GET /price/XAU` | Không | Unlimited, update mỗi 30s |
| Twelve Data | `GET /time_series` | `TWELVEDATA_API_KEY` | Free: 800 credits/ngày |

#### Hàm

**`FetchLiveGoldPrice() (*PriceAPIResponse, error)`**
- Gọi `https://api.gold-api.com/price/XAU`, timeout 15s
- Trả giá spot hiện tại, không cần API key

**`AppendLiveCandle(records []PriceRecord, interval string) ([]PriceRecord, error)`**
- Gọi `FetchLiveGoldPrice()` lấy giá real-time
- Tính slot nến hiện tại bằng round-down theo `interval`
- Open = close nến lịch sử cuối; High/Low = max/min(open, livePrice)
- Date có hậu tố `⟳`, timezone ICT (UTC+7)

**`FetchHistoricalOHLC(symbol, interval string, outputsize int) ([]PriceRecord, error)`**
- Gọi Twelve Data `/time_series?timezone=Asia/Ho_Chi_Minh`
- Trả dữ liệu theo giờ Việt Nam
- Tối đa 5000 nến, 1 credit/request

**`FetchLiveOHLCHistory(numCandles int, candleDuration, pollInterval time.Duration) ([]PriceRecord, error)`**
- Poll gold-api.com mỗi `pollInterval` trong `candleDuration` để xây nến OHLC
- Tổng thời gian: `numCandles × candleDuration`

**`candleIntervalMinutes(interval string) int`**
- Convert `"15min"` → 15, `"1h"` → 60, v.v.

---

### `llm.go`

**`LLMClient`**
- Wrapper cho bất kỳ endpoint tương thích OpenAI chat completion
- HTTP timeout: 120 giây (reasoning model cần thời gian dài)

**`Predict(history []PriceRecord) (PredictionResult, error)`**
- Gọi `buildPrompt()` → POST đến `{BaseURL}/chat/completions`
- Temperature: `0.3`, MaxTokens: `4000`
- Retry tối đa 3 lần, backoff `2s / 4s / 8s` khi gặp 429 hoặc 5xx
- Retry khi response rỗng (model free bị giới hạn token)
- Hỗ trợ reasoning model (Nemotron, v.v.): trích xuất JSON từ `reasoning` field khi `content` là null
- Fallback tự tính `TradeSetup` từ `KeyLevels` nếu model không trả `trade_setup`

**`buildPrompt(history []PriceRecord) string`**
- Lấy tối đa **30 nến gần nhất** (tránh vượt token limit)
- Format bảng OHLC + giá hiện tại
- Yêu cầu LLM trả JSON compact có đủ: `direction`, `confidence`, `predicted_price`, `reasoning`, `key_levels`, `trade_setup` (type, entry, stop_loss, take_profit_1, take_profit_2, risk_reward)

**`deriveTradeSetup(direction, predictedPrice, support, resistance)`**
- Fallback khi model không trả `trade_setup`
- BUY: entry = support + spread, SL = support - buffer, TP1 = resistance - spread
- SELL: entry = resistance - spread, SL = resistance + buffer, TP1 = support + spread
- Tự tính R:R = (TP1 - entry) / (entry - SL)

**`extractJSONFromReasoning(reasoning string) string`**
- Tìm JSON block cuối cùng trong reasoning text của reasoning model
- Kiểm tra có chứa key `"direction"` để xác nhận đúng block

**`stripCodeFences(s string) string`**
- Xoá ` ```json ` / ` ``` ` wrapper

---

### `chart.go`

**`generateChartHTML(opts ChartOptions) error`**
- Convert `[]PriceRecord` → JSON array Unix timestamp
- Nhận diện nến live bằng `⟳` → lưu `liveIdx`
- Gọi `chartTemplate()` → ghi HTML → mở browser

**`parseChartTime(dateStr string) (int64, error)`**
- Strip `⟳`, thử 4 format: `2006-01-02 15:04:05`, `2006-01-02 15:04`, `2006-01-02`, RFC3339

**`chartTemplate(title, candlesJSON string, liveIdx int) string`**
- TradingView Lightweight Charts v4.1.3 (CDN `unpkg.com`)
- Nến đã đóng: cyan `#00bcd4` (tăng) / đỏ `#e53935` (giảm)
- Nến live (`liveIdx ≥ 0`): vàng `#ffd700`, badge `● LIVE` nhấp nháy
- Nền đen `#000000`, grid dashed `#1a1a1a`
- Crosshair tooltip: O/H/L/C + % thay đổi, label `⟳ forming` cho nến live
- Label giá cuối bám trục phải (màu theo chiều nến)
- Full screen, responsive resize

**`openBrowser(path string)`**
- macOS: `open file://...` | Windows: `rundll32` | Linux: `xdg-open`

---

### `telegram.go`

**`TelegramConfig`** — `BotToken`, `ChatID` đọc từ env.

**`SendTelegramMessage(cfg, text string) error`**
- POST đến `https://api.telegram.org/bot{TOKEN}/sendMessage`
- parse_mode: HTML, timeout 15s

**`FormatTelegramMessage(r PredictionResult, currentPrice float64, interval string, t time.Time) string`**
- Định dạng HTML với emoji, bold, code blocks
- Gồm: giá hiện tại, xu hướng, tin cậy, dự đoán, S/R, trade setup (entry/SL/TP1/TP2/R:R), phân tích

**`RunWatchLoop(cfg, tgCfg, histInterval, histSize, watchInterval, minConfidence)`**
- Chạy `runOnce()` ngay lập tức, sau đó lặp theo `watchInterval`
- `runOnce()`: fetch → live candle → predict → print → gửi Telegram nếu `confidence ≥ minConfidence`
- In terminal: kết quả mỗi lần; gửi Telegram: chỉ khi đạt ngưỡng

**`intervalLabel(interval string) string`**
- Convert `"15min"` → `"15"`, `"1h"` → `"60"`, `"1day"` → `"D1"` cho tiêu đề Telegram

---

## Cấu hình môi trường

| Biến | Bắt buộc | Mô tả |
|---|---|---|
| `LLM_API_KEY` | ✅ | API key LLM |
| `LLM_BASE_URL` | ❌ | Base URL (mặc định: `https://api.openai.com/v1`) |
| `LLM_MODEL` | ❌ | Tên model (mặc định: `gpt-4o-mini`) |
| `TWELVEDATA_API_KEY` | Chỉ với `-history` / `-watch` | API key Twelve Data |
| `TELEGRAM_BOT_TOKEN` | Chỉ với `-watch` | Token bot Telegram |
| `TELEGRAM_CHAT_ID` | Chỉ với `-watch` | Chat ID nhận tin |

---

## Tất cả CLI flags

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-history` | false | Lấy lịch sử OHLC từ Twelve Data |
| `-history-size` | 100 | Số nến (1–5000) |
| `-history-interval` | `15min` | Khung nến: `1min` `5min` `15min` `30min` `45min` `1h` `2h` `4h` `1day` |
| `-chart` | false | Tạo biểu đồ HTML MT5-style |
| `-chart-out` | `chart.html` | Đường dẫn file biểu đồ |
| `-watch` | false | Watch mode: lặp phân tích + Telegram |
| `-watch-interval` | `3m` | Tần suất trong watch mode |
| `-min-confidence` | `0.75` | Ngưỡng tin cậy tối thiểu để gửi Telegram |
| `-live` | false | Thu thập nến real-time bằng polling |
| `-live-snapshots` | 10 | Số nến cần thu thập |
| `-live-candle` | `15m` | Thời gian mỗi nến |
| `-live-interval` | `30s` | Tần suất poll trong mỗi nến |
| `-data` | — | Đường dẫn file JSON OHLC |
| `-i` | false | Đọc JSON từ stdin |

---

## Xử lý lỗi và giới hạn đã biết

| Tình huống | Xử lý |
|---|---|
| LLM trả `content: null` (reasoning model) | `extractJSONFromReasoning()` tìm JSON trong `reasoning` field |
| LLM trả response rỗng | Retry 3 lần với backoff |
| LLM trả JSON thiếu `trade_setup` | `deriveTradeSetup()` tự tính từ `key_levels` |
| LLM bọc JSON trong code fence | `stripCodeFences()` xử lý |
| Không lấy được nến live | Warning, tiếp tục với data lịch sử |
| `.env` không tồn tại | Bỏ qua, không crash |
| Date không parse được | Fallback `time.Now().Unix()` |
| Telegram gửi thất bại | Log lỗi, không crash |

---

## Phụ thuộc runtime

| Thư viện | File | Cách load |
|---|---|---|
| TradingView Lightweight Charts v4.1.3 | `chart.go` (nhúng trong HTML) | CDN `unpkg.com` |

Không có Go module dependency ngoài standard library.
