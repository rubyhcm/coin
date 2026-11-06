# CoinPredict — Tài liệu kỹ thuật

## Tổng quan

CoinPredict là ứng dụng CLI viết bằng Go, lấy dữ liệu giá vàng XAU/USD, vẽ biểu đồ nến kiểu MT5 và gọi LLM (tương thích OpenAI) để dự đoán xu hướng giá kỳ tiếp theo.

---

## Kiến trúc

```
main.go      — CLI, routing chế độ, orchestration
price.go     — Lấy dữ liệu giá (gold-api.com, Twelve Data)
llm.go       — Gọi LLM API, build prompt, parse kết quả
chart.go     — Tạo file HTML biểu đồ nến MT5-style
```

Không có dependency ngoài standard library của Go — toàn bộ dùng `net/http`, `encoding/json`, `os`, `flag`.

---

## Luồng xử lý

```
main()
 ├── loadDotEnv(".env")          # tự load biến môi trường
 ├── parse flags
 ├── LoadConfig()                # đọc LLM_API_KEY, LLM_BASE_URL, LLM_MODEL
 │
 ├── [switch chế độ dữ liệu]
 │    ├── -history  → FetchHistoricalOHLC() + AppendLiveCandle()
 │    ├── -live     → FetchLiveOHLCHistory()
 │    ├── -data     → loadPriceData()
 │    ├── -i        → readPriceDataStdin()
 │    └── (default) → generateDemoData()
 │
 ├── [-chart] → generateChartHTML()  # tạo chart.html, mở browser
 │
 └── LLMClient.Predict()             # gửi 30 nến gần nhất cho LLM
      └── printPrediction()
```

---

## Các file và hàm chính

### `main.go`

| Hàm / Type | Mô tả |
|---|---|
| `loadDotEnv(path)` | Đọc file `.env`, set env vars. Shell env luôn được ưu tiên (không overwrite). |
| `Config` | Struct chứa `APIKey`, `BaseURL`, `Model` cho LLM. |
| `PriceRecord` | Struct OHLC: `Date`, `Open`, `High`, `Low`, `Close`, `Volume`. Date có thể chứa ký hiệu `⟳` để đánh dấu nến đang hình thành. |
| `PredictionResult` | Struct kết quả LLM: `Direction`, `Confidence`, `PredictedPrice`, `Reasoning`, `KeyLevels{Support, Resistance}`. |
| `main()` | Điểm vào. Parse flags, load data, gọi chart và LLM. |
| `printPrediction(r)` | In kết quả dự đoán ra stdout theo định dạng bảng. |
| `loadPriceData(path)` | Đọc JSON file → `[]PriceRecord`. |
| `readPriceDataStdin()` | Đọc JSON từ stdin (chế độ `-i`). |
| `generateDemoData()` | Tạo 12 bản ghi giá mẫu cho demo. |
| `getEnv(key, fallback)` | Đọc env var, trả fallback nếu rỗng. |

---

### `price.go`

#### Nguồn dữ liệu

| Nguồn | API | Auth | Giới hạn |
|---|---|---|---|
| gold-api.com | `GET /price/XAU` | Không cần | Unlimited, update mỗi 30s |
| Twelve Data | `GET /time_series` | `TWELVEDATA_API_KEY` | Free: 800 credits/ngày |

#### Hàm

**`FetchLiveGoldPrice() (*PriceAPIResponse, error)`**
- Gọi `https://api.gold-api.com/price/XAU`
- Trả về giá spot hiện tại, không cần API key
- Timeout: 15 giây

**`AppendLiveCandle(records []PriceRecord, interval string) ([]PriceRecord, error)`**
- Gọi `FetchLiveGoldPrice()` lấy giá hiện tại
- Tính slot nến hiện tại bằng cách round-down thời gian theo `interval`
- Open = close của nến lịch sử cuối cùng
- Date có hậu tố `⟳` để chart nhận biết đây là nến đang hình thành
- Timezone cố định: ICT (UTC+7)

**`candleIntervalMinutes(interval string) int`**
- Chuyển đổi chuỗi interval (`"15min"`, `"1h"`, v.v.) → số phút
- Dùng để tính slot boundary cho nến live

**`FetchHistoricalOHLC(symbol, interval string, outputsize int) ([]PriceRecord, error)`**
- Gọi Twelve Data `/time_series?symbol=XAU/USD&interval=...&timezone=Asia/Ho_Chi_Minh`
- Kết quả trả về theo giờ Việt Nam (ICT, UTC+7)
- Tối đa 5000 nến mỗi request (1 API credit/request)
- Parse response JSON → `[]PriceRecord`

**`FetchLiveOHLCHistory(numCandles int, candleDuration, pollInterval time.Duration) ([]PriceRecord, error)`**
- Thu thập nến OHLC bằng cách poll gold-api.com theo chu kỳ `pollInterval` trong suốt `candleDuration`
- Mỗi nến: poll nhiều lần → track open/high/low, close = giá cuối cùng trong nến
- Tổng thời gian: `numCandles × candleDuration`

---

### `llm.go`

**`LLMClient`**
- Wrapper cho bất kỳ endpoint tương thích OpenAI chat completion
- HTTP timeout: 60 giây

**`Predict(history []PriceRecord) (PredictionResult, error)`**
- Gọi `buildPrompt()` → tạo prompt
- POST đến `{BaseURL}/chat/completions`
- Temperature: `0.3` (low, để kết quả nhất quán)
- MaxTokens: `4000`
- Retry logic: tối đa 3 lần, backoff `2s / 4s / 8s` khi gặp 429 hoặc 5xx
- Retry thêm khi model trả response rỗng (phổ biến với free tier)
- Strip code fences (` ```json ``` `) trước khi parse JSON

**`buildPrompt(history []PriceRecord) string`**
- Chỉ lấy **30 nến gần nhất** để gửi LLM (tránh vượt token limit của free model)
- Format bảng markdown OHLC
- Yêu cầu LLM trả JSON compact (1 dòng) để dễ parse

**`stripCodeFences(s string) string`**
- Xoá ` ```json ` và ` ``` ` wrapper nếu model bọc response trong code block

---

### `chart.go`

**`generateChartHTML(opts ChartOptions) error`**
- Duyệt `[]PriceRecord`, convert datetime → Unix timestamp (hàm `parseChartTime`)
- Nhận diện nến live bằng ký hiệu `⟳` trong Date → ghi nhớ `liveIdx`
- Serialize candles → JSON
- Gọi `chartTemplate()` → ghi file HTML → mở browser (nếu `AutoOpen`)

**`parseChartTime(dateStr string) (int64, error)`**
- Strip ký hiệu `⟳` trước khi parse
- Hỗ trợ 4 format: `2006-01-02 15:04:05`, `2006-01-02 15:04`, `2006-01-02`, RFC3339

**`chartTemplate(title, candlesJSON string, liveIdx int) string`**
- Nhúng TradingView Lightweight Charts v4.1.3 (CDN)
- Nến đã đóng: cyan (`#00bcd4`) tăng / đỏ (`#e53935`) giảm — đúng palette MT5
- Nến đang hình thành (`liveIdx ≥ 0`): vàng (`#ffd700`), badge `● LIVE` nhấp nháy
- Nền đen tuyệt đối (`#000000`), grid dashed `#1a1a1a`
- Crosshair tooltip: O/H/L/C + % thay đổi, nhãn `⟳ forming` cho nến live
- Label giá cuối bám trục phải, màu theo chiều nến

**`openBrowser(path string)`**
- macOS: `open file://...`
- Windows: `rundll32 url.dll,FileProtocolHandler`
- Linux: `xdg-open`

---

## Cấu hình môi trường

Đọc từ file `.env` (auto-load) hoặc shell environment. Shell env được ưu tiên.

| Biến | Bắt buộc | Mô tả |
|---|---|---|
| `LLM_API_KEY` | ✅ | API key của nhà cung cấp LLM |
| `LLM_BASE_URL` | ❌ | Base URL endpoint (mặc định: `https://api.openai.com/v1`) |
| `LLM_MODEL` | ❌ | Tên model (mặc định: `gpt-4o-mini`) |
| `TWELVEDATA_API_KEY` | Chỉ với `-history` | API key Twelve Data |

---

## Kiểu dữ liệu trung tâm

```go
// PriceRecord — một nến OHLC
type PriceRecord struct {
    Date   string  // "2006-01-02 15:04:05" hoặc "2006-01-02 15:04:05 ⟳" (nến live)
    Open   float64
    High   float64
    Low    float64
    Close  float64
    Volume float64
}

// PredictionResult — kết quả phân tích từ LLM
type PredictionResult struct {
    Direction      string  // "up" | "down" | "sideways"
    Confidence     float64 // 0.0 – 1.0
    PredictedPrice float64
    Reasoning      string
    KeyLevels      struct {
        Support    float64
        Resistance float64
    }
}
```

---

## Xử lý lỗi và giới hạn đã biết

| Tình huống | Xử lý |
|---|---|
| LLM trả response rỗng | Retry tối đa 3 lần với backoff |
| LLM bọc JSON trong code fence | `stripCodeFences()` tự xử lý |
| gold-api.com timeout | HTTP timeout 15s, báo lỗi rõ |
| Twelve Data lỗi (key sai, hết credit) | Parse `status=error` + `message`, log và exit |
| Nến live không lấy được | Warning, vẫn tiếp tục với data lịch sử |
| `.env` không tồn tại | Bỏ qua, không crash |
| Date không parse được | Fallback `time.Now().Unix()` |

---

## Phụ thuộc ngoài (runtime)

| Thư viện | Dùng ở | Cách load |
|---|---|---|
| TradingView Lightweight Charts v4.1.3 | `chart.go` (HTML) | CDN `unpkg.com` |

Không có Go module dependency nào ngoài standard library.
