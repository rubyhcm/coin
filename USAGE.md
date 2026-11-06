# CoinPredict — Hướng dẫn sử dụng

## Yêu cầu

- Go 1.21+
- Kết nối internet
- API key LLM (OpenAI, Groq, OpenRouter, Ollama, v.v.)
- API key Twelve Data (chỉ cần cho chế độ `-history`) — miễn phí

---

## Cài đặt

```bash
# 1. Clone hoặc tải source
cd /path/to/coin

# 2. Tạo file .env
cp .env.example .env

# 3. Điền thông tin vào .env (xem phần Cấu hình bên dưới)

# 4. Build
go build -o coinpredict .
```

---

## Cấu hình (.env)

```env
# === LLM ===
LLM_API_KEY=sk-...               # API key của nhà cung cấp LLM (bắt buộc)
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4o-mini

# === Dữ liệu lịch sử ===
TWELVEDATA_API_KEY=...            # Lấy miễn phí tại https://twelvedata.com/pricing
```

### Ví dụ theo từng nhà cung cấp LLM

**OpenAI:**
```env
LLM_API_KEY=sk-proj-...
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4o-mini
```

**Groq (miễn phí, nhanh):**
```env
LLM_API_KEY=gsk_...
LLM_BASE_URL=https://api.groq.com/openai/v1
LLM_MODEL=llama-3.3-70b-versatile
```

**OpenRouter (nhiều model, có free tier):**
```env
LLM_API_KEY=sk-or-...
LLM_BASE_URL=https://openrouter.ai/api/v1
LLM_MODEL=google/gemma-3-27b-it:free
```

**Ollama (chạy local, không cần internet):**
```env
LLM_API_KEY=ollama
LLM_BASE_URL=http://localhost:11434/v1
LLM_MODEL=llama3.1
```

> App tự đọc `.env` khi khởi động — không cần `source .env`.

---

## Các chế độ sử dụng

### 1. Chế độ lịch sử — khuyên dùng (`-history`)

Lấy nến OHLC lịch sử XAU/USD từ Twelve Data + tự động ghép nến đang hình thành.

```bash
# Mặc định: 100 nến 15 phút, chỉ hiện dự đoán LLM
./coinpredict -history

# 100 nến 15 phút + vẽ biểu đồ MT5
./coinpredict -history -chart

# 200 nến 1 giờ + biểu đồ
./coinpredict -history -history-size 200 -history-interval 1h -chart

# 50 nến 30 phút + biểu đồ, lưu file tên riêng
./coinpredict -history -history-size 50 -history-interval 30min -chart -chart-out gold_30m.html
```

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-history` | false | Bật chế độ lịch sử |
| `-history-size` | 100 | Số nến cần lấy (1–5000) |
| `-history-interval` | `15min` | Khung thời gian: `1min` `5min` `15min` `30min` `45min` `1h` `2h` `4h` `1day` |

**Lưu ý:**
- Nến cuối cùng là nến **đang hình thành** (màu vàng trên biểu đồ), lấy giá real-time từ gold-api.com
- Dữ liệu hiển thị theo **giờ Việt Nam** (ICT, UTC+7)
- Cần `TWELVEDATA_API_KEY` trong `.env`

---

### 2. Chế độ real-time (`-live`)

Thu thập nến OHLC trực tiếp bằng cách poll giá liên tục từ gold-api.com (không cần API key).

```bash
# 10 nến × 15 phút, poll mỗi 30 giây (~2.5 giờ)
./coinpredict -live

# 5 nến × 1 giờ, poll mỗi 1 phút (~5 giờ)
./coinpredict -live -live-snapshots 5 -live-candle 1h -live-interval 1m

# Test nhanh: 3 nến × 30 giây
./coinpredict -live -live-snapshots 3 -live-candle 30s -live-interval 10s -chart
```

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-live` | false | Bật chế độ real-time |
| `-live-snapshots` | 10 | Số nến OHLC cần thu thập |
| `-live-candle` | `15m` | Thời gian mỗi nến (`15m`, `30m`, `1h`, v.v.) |
| `-live-interval` | `30s` | Tần suất poll giá trong mỗi nến |

**Lưu ý:** Tổng thời gian = `live-snapshots × live-candle`. Chế độ này chỉ nên dùng khi muốn xây nến từ dữ liệu thô, không qua Twelve Data.

---

### 3. Biểu đồ nến MT5 (`-chart`)

Thêm `-chart` vào bất kỳ lệnh nào để tạo biểu đồ HTML và tự mở browser.

```bash
./coinpredict -history -chart
./coinpredict -history -history-size 200 -history-interval 1h -chart
./coinpredict -live -live-snapshots 5 -live-candle 30s -live-interval 10s -chart
```

Biểu đồ bao gồm:
- Nền đen, nến **cyan** (tăng) / **đỏ** (giảm) — đúng palette MT5
- Nến đang hình thành: màu **vàng** + badge `● LIVE` nhấp nháy
- Crosshair tooltip: O / H / L / C + % thay đổi khi di chuột
- Label giá cuối bám trục phải
- Zoom/pan bằng chuột

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-chart` | false | Bật vẽ biểu đồ HTML |
| `-chart-out` | `chart.html` | Đường dẫn file output |

---

### 4. Từ file JSON (`-data`)

Dùng file JSON chứa dữ liệu OHLC tự chuẩn bị.

```bash
./coinpredict -data my_data.json
./coinpredict -data my_data.json -chart
```

**Định dạng file:**
```json
[
  {"date": "2026-11-01", "open": 4020.0, "high": 4045.0, "low": 4015.0, "close": 4038.0},
  {"date": "2026-11-02", "open": 4038.0, "high": 4060.0, "low": 4030.0, "close": 4055.0}
]
```

---

### 5. Chế độ tương tác (`-i`)

Dán JSON trực tiếp vào terminal.

```bash
./coinpredict -i
# Dán JSON array, nhấn Enter, rồi Ctrl+D để kết thúc
```

---

### 6. Demo (không flag)

Chạy với dữ liệu mẫu tĩnh — không cần Twelve Data, không cần internet (ngoài LLM).

```bash
./coinpredict
./coinpredict -chart
```

---

## Kết quả dự đoán

```
════════════════════════════════════════════
       GOLD/USD PRICE PREDICTION
════════════════════════════════════════════
Direction        : UP
Confidence       : 72%
Predicted Price  : 4048.50 USD
Support Level    : 4040.00 USD
Resistance Level : 4050.00 USD
──────────────────────────────────────────────
Analysis:
Price bounced off strong support at 4040.00 with five consecutive
higher closes, shifting short-term momentum upward...
════════════════════════════════════════════
⚠️  DISCLAIMER: This is AI-generated analysis, not financial advice.
```

| Trường | Mô tả |
|---|---|
| Direction | `UP` tăng / `DOWN` giảm / `SIDEWAYS` đi ngang |
| Confidence | Độ tin cậy 0–100% |
| Predicted Price | Giá dự đoán kỳ tiếp theo (USD/oz) |
| Support Level | Mức hỗ trợ gần nhất |
| Resistance Level | Mức kháng cự gần nhất |
| Analysis | Phân tích ngắn gọn từ LLM |

---

## Lệnh hay dùng

```bash
# Lệnh chuẩn hàng ngày: 100 nến 15 phút + biểu đồ + dự đoán
./coinpredict -history -history-size 100 -history-interval 15min -chart

# Xem xu hướng ngắn hạn hơn (30 nến gần nhất)
./coinpredict -history -history-size 30 -history-interval 15min -chart

# Phân tích khung 1 giờ
./coinpredict -history -history-size 100 -history-interval 1h -chart

# Chỉ xem dự đoán, không mở browser
./coinpredict -history -history-size 100 -history-interval 15min

# Lưu biểu đồ vào file tên riêng
./coinpredict -history -chart -chart-out xauusd_m15.html
```

---

## Xử lý lỗi thường gặp

| Lỗi | Nguyên nhân | Cách xử lý |
|---|---|---|
| `LLM_API_KEY not set` | Chưa có file `.env` hoặc thiếu key | Tạo `.env` từ `.env.example`, điền key |
| `TWELVEDATA_API_KEY not set` | Thiếu key Twelve Data | Đăng ký miễn phí tại twelvedata.com |
| `Prediction failed: empty response` | Model free hết token hoặc giới hạn | Thử model khác hoặc giảm `-history-size` |
| `Twelve Data error: ...` | Key sai, hết credit (>800/ngày) | Kiểm tra key, chờ reset lúc 00:00 UTC |
| `gold API returned status 429` | Rate limit gold-api.com | Tăng `-live-interval` lên ≥30s |

---

## Lưu ý

- Dữ liệu lịch sử Twelve Data hiển thị theo **giờ Việt Nam** (ICT, UTC+7)
- LLM chỉ nhận **30 nến gần nhất** để phân tích (tránh vượt token limit của model free)
- Biểu đồ hiển thị **toàn bộ** số nến đã lấy (không giới hạn 30)
- Đây là phân tích do AI tạo ra, **không phải lời khuyên tài chính**
