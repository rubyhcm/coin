# CoinPredict — Hướng dẫn sử dụng

## Yêu cầu

- Go 1.21+
- Kết nối internet
- API key LLM (OpenAI, Groq, OpenRouter, Ollama, v.v.)
- API key Twelve Data (chỉ cần cho `-history`) — miễn phí
- Telegram Bot Token + Chat ID (chỉ cần cho `-watch`) — miễn phí

---

## Cài đặt

```bash
cp .env.example .env   # tạo file cấu hình
# điền các API key vào .env
go build -o coinpredict .
```

---

## Cấu hình (.env)

App tự đọc file `.env` khi khởi động — không cần `source .env`.
Shell environment luôn được ưu tiên hơn (không bị overwrite).

```env
# === LLM (bắt buộc) ===
LLM_API_KEY=sk-...
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4o-mini

# === Dữ liệu lịch sử (chỉ cần cho -history) ===
TWELVEDATA_API_KEY=...       # miễn phí tại https://twelvedata.com/pricing

# === Telegram (chỉ cần cho -watch) ===
TELEGRAM_BOT_TOKEN=...       # lấy từ @BotFather
TELEGRAM_CHAT_ID=...         # lấy từ @userinfobot
```

### Ví dụ theo nhà cung cấp LLM

**OpenRouter — model free (khuyên dùng):**
```env
LLM_API_KEY=sk-or-...
LLM_BASE_URL=https://openrouter.ai/api/v1
LLM_MODEL=nvidia/nemotron-3-ultra-550b-a55b:free
```

**Groq (nhanh, free tier):**
```env
LLM_API_KEY=gsk_...
LLM_BASE_URL=https://api.groq.com/openai/v1
LLM_MODEL=llama-3.3-70b-versatile
```

**Ollama (local, không cần internet cho LLM):**
```env
LLM_API_KEY=ollama
LLM_BASE_URL=http://localhost:11434/v1
LLM_MODEL=llama3.1
```

---

## Các chế độ sử dụng

### 1. Chế độ lịch sử (`-history`) — khuyên dùng

Lấy nến OHLC lịch sử XAU/USD từ Twelve Data + tự động ghép nến đang hình thành.

```bash
# 100 nến 15 phút + dự đoán LLM (không biểu đồ)
./coinpredict -history

# 100 nến 15 phút + biểu đồ MT5 + dự đoán
./coinpredict -history -chart

# 200 nến 1 giờ + biểu đồ
./coinpredict -history -history-size 200 -history-interval 1h -chart

# 50 nến 30 phút, lưu biểu đồ tên riêng
./coinpredict -history -history-size 50 -history-interval 30min -chart -chart-out gold_30m.html
```

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-history` | false | Bật chế độ lịch sử |
| `-history-size` | 100 | Số nến cần lấy (1–5000) |
| `-history-interval` | `15min` | Khung thời gian: `1min` `5min` `15min` `30min` `45min` `1h` `2h` `4h` `1day` |

**Lưu ý:**
- Nến cuối là nến **đang hình thành** (giá real-time từ gold-api.com), màu vàng trên biểu đồ
- Thời gian hiển thị theo **giờ Việt Nam** (ICT, UTC+7)
- Cần `TWELVEDATA_API_KEY` trong `.env`

---

### 2. Chế độ watch — Telegram bot (`-watch`)

Phân tích tự động định kỳ và gửi kết quả lên Telegram. Chỉ gửi khi độ tin cậy đạt ngưỡng tối thiểu.

```bash
# Mỗi 3 phút, gửi khi tin cậy ≥ 75% (mặc định)
./coinpredict -watch

# Mỗi 5 phút, ngưỡng 80%
./coinpredict -watch -watch-interval 5m -min-confidence 0.8

# Tùy chỉnh đầy đủ
./coinpredict -watch -history-size 50 -history-interval 15min -watch-interval 3m -min-confidence 0.75
```

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-watch` | false | Bật watch mode |
| `-watch-interval` | `3m` | Tần suất phân tích (vd: `3m`, `5m`, `10m`) |
| `-min-confidence` | `0.75` | Ngưỡng tin cậy tối thiểu để gửi Telegram (0.0–1.0) |

**Cách thiết lập Telegram bot:**
1. Chat với [@BotFather](https://t.me/botfather) → `/newbot` → nhận `BOT_TOKEN`
2. Chat với [@userinfobot](https://t.me/userinfobot) → nhận `CHAT_ID`
3. Điền vào `.env`:
   ```
   TELEGRAM_BOT_TOKEN=123456:ABC...
   TELEGRAM_CHAT_ID=987654321
   ```

Terminal luôn in kết quả mỗi lần phân tích. Telegram chỉ nhận tin khi confidence ≥ ngưỡng.

---

### 3. Biểu đồ nến MT5 (`-chart`)

Thêm `-chart` vào bất kỳ lệnh nào để tạo biểu đồ HTML và tự mở browser.

```bash
./coinpredict -history -chart
./coinpredict -history -history-size 200 -history-interval 1h -chart -chart-out xauusd_h1.html
```

| Flag | Mặc định | Mô tả |
|---|---|---|
| `-chart` | false | Tạo biểu đồ HTML và mở browser |
| `-chart-out` | `chart.html` | Đường dẫn file output |

Biểu đồ bao gồm:
- Nền đen, nến **cyan** (tăng) / **đỏ** (giảm) — palette MT5
- Nến đang hình thành: màu **vàng** + badge `● LIVE` nhấp nháy
- Crosshair tooltip: O / H / L / C + % thay đổi
- Label giá cuối bám trục phải
- Zoom / pan bằng chuột, full screen

---

### 4. Từ file JSON (`-data`)

```bash
./coinpredict -data my_data.json -chart
```

**Định dạng:**
```json
[
  {"date": "2026-11-01", "open": 4020.0, "high": 4045.0, "low": 4015.0, "close": 4038.0},
  {"date": "2026-11-02", "open": 4038.0, "high": 4060.0, "low": 4030.0, "close": 4055.0}
]
```

---

### 5. Các chế độ khác

| Lệnh | Mô tả |
|---|---|
| `./coinpredict` | Demo với dữ liệu mẫu tĩnh |
| `./coinpredict -i` | Dán JSON từ stdin, nhấn Ctrl+D để kết thúc |
| `./coinpredict -live` | Thu thập nến OHLC real-time bằng polling |

---

## Kết quả dự đoán

```
════════════════════════════════════════════
       GOLD/USD PRICE PREDICTION
════════════════════════════════════════════
Direction        : DOWN
Confidence       : 65%
Predicted Price  : 4045.00 USD
Support Level    : 4037.00 USD
Resistance Level : 4050.50 USD
────────────────────────────────────────────
Trade Setup      : SELL
Entry            : 4049.50 USD
Stop Loss        : 4052.50 USD
Take Profit 1    : 4042.00 USD
Take Profit 2    : 4037.00 USD
Risk/Reward      : 1 : 2.50
────────────────────────────────────────────
Analysis:
Price collapsed from 4106 to 4036...
════════════════════════════════════════════
⚠️  DISCLAIMER: This is AI-generated analysis, not financial advice.
```

| Trường | Mô tả |
|---|---|
| Direction | `UP` tăng / `DOWN` giảm / `SIDEWAYS` đi ngang |
| Confidence | Độ tin cậy 0–100% |
| Predicted Price | Giá dự đoán kỳ tiếp theo (USD/oz) |
| Support / Resistance | Mức hỗ trợ / kháng cự gần nhất |
| Trade Setup | `BUY` / `SELL` / `WAIT` |
| Entry | Giá vào lệnh đề xuất |
| Stop Loss | Mức dừng lỗ |
| Take Profit 1/2 | Mục tiêu lợi nhuận 1 (thận trọng) và 2 (mở rộng) |
| Risk/Reward | Tỷ lệ rủi ro/lợi nhuận đến TP1 |

---

## Lệnh hay dùng

```bash
# Phân tích + biểu đồ ngay lập tức
./coinpredict -history -history-size 100 -history-interval 15min -chart

# Chỉ xem dự đoán + trade setup, không mở browser
./coinpredict -history -history-size 100 -history-interval 15min

# Chạy bot Telegram tự động mỗi 3 phút
./coinpredict -watch -history-size 50 -history-interval 15min

# Phân tích khung H1
./coinpredict -history -history-size 100 -history-interval 1h -chart
```

---

## Xử lý lỗi thường gặp

| Lỗi | Nguyên nhân | Cách xử lý |
|---|---|---|
| `LLM_API_KEY not set` | Chưa có `.env` hoặc thiếu key | Tạo `.env` từ `.env.example` |
| `TWELVEDATA_API_KEY not set` | Thiếu key Twelve Data | Đăng ký miễn phí tại twelvedata.com |
| `TELEGRAM_BOT_TOKEN ... must be set` | Thiếu config Telegram | Điền `TELEGRAM_BOT_TOKEN` và `TELEGRAM_CHAT_ID` vào `.env` |
| `empty response from model` | Model free hết token | Đổi model hoặc giảm `-history-size` |
| `429 rate-limited upstream` | Model Google/shared bị giới hạn | Chờ vài phút hoặc đổi sang `nvidia/nemotron-3-ultra-550b-a55b:free` |
| `Twelve Data error` | Key sai, hết 800 credit/ngày | Kiểm tra key, chờ reset 00:00 UTC |

---

## Lưu ý

- Dữ liệu lịch sử Twelve Data hiển thị theo **giờ Việt Nam** (ICT, UTC+7)
- LLM chỉ nhận **30 nến gần nhất** để phân tích (tránh vượt token limit)
- Biểu đồ hiển thị **toàn bộ** số nến đã lấy
- Đây là phân tích do AI tạo ra, **không phải lời khuyên tài chính**
