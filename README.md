# CoinPredict — Dự đoán giá Gold/USD bằng LLM

Ứng dụng Go gọi API LLM (tương thích OpenAI) để phân tích dữ liệu giá vàng (XAU/USD) và dự đoán xu hướng giá kỳ tiếp theo.

## Yêu cầu

- Go 1.21+ (đã cài Go 1.27)
- API key từ một nhà cung cấp LLM tương thích OpenAI: OpenAI, Groq, OpenRouter, Ollama, v.v.

## Cài đặt

```bash
# 1. Tạo file .env từ mẫu
cp .env.example .env

# 2. Điền API key vào .env
#    OpenAI:      LLM_API_KEY=sk-...
#    Groq (free): LLM_API_KEY=gsk_...  LLM_BASE_URL=https://api.groq.com/openai/v1  LLM_MODEL=llama-3.3-70b-versatile
#    Ollama:      LLM_API_KEY=ollama   LLM_BASE_URL=http://localhost:11434/v1    LLM_MODEL=llama3.1
```

## Sử dụng

```bash
# Build
go build -o coinpredict .

# Chạy với dữ liệu mẫu (demo)
./coinpredict

# Chạy với file JSON chứa dữ liệu giá
./coinpredict -data sample_data.json

# Chế độ tương tác: dán JSON vào stdin
./coinpredict -i

# Chế độ REAL-TIME: lấy giá vàng trực tiếp từ API
./coinpredict -live
# Lấy 20 nến 15 phút, poll mỗi 30 giây
./coinpredict -live -live-snapshots 20 -live-candle 15m -live-interval 30s

# Chế độ LỊCH SỬ: lấy nến OHLC 15 phút từ Twelve Data
./coinpredict -history
# 100 nến 15 phút + vẽ biểu đồ
./coinpredict -history -history-size 100 -history-interval 15min -chart
# 50 nến 1 giờ
./coinpredict -history -history-size 50 -history-interval 1h -chart
```

### Chế độ Real-time (`-live`)

Lấy giá XAU/USD trực tiếp từ [gold-api.com](https://gold-api.com) (miễn phí, không cần API key, không giới hạn request). App thu thập dữ liệu thành các nến OHLC (mỗi nến kéo dài `live-candle`), poll giá liên tục trong nến để xác định open/high/low/close, sau đó gửi cho LLM phân tích.

| Flag | Mặc định | Mô tả |
|------|----------|-------|
| `-live` | false | Bật chế độ real-time |
| `-live-snapshots` | 10 | Số lượng nến OHLC cần thu thập |
| `-live-candle` | 15m | Thời gian mỗi nến (vd: `15m`, `30m`, `1h`) |
| `-live-interval` | 30s | Khoảng thời gian poll giá trong mỗi nến |

### Chế độ Lịch sử (`-history`)

Lấy nến OHLC lịch sử XAU/USD từ [Twelve Data](https://twelvedata.com) (free 800 credits/ngày). Cần `TWELVEDATA_API_KEY` trong file `.env` (đăng ký miễn phí tại https://twelvedata.com/pricing).

| Flag | Mặc định | Mô tả |
|------|----------|-------|
| `-history` | false | Bật chế độ lấy dữ liệu lịch sử |
| `-history-interval` | 15min | Khung thời gian nến (`15min`, `30min`, `1h`, `1day`) |
| `-history-size` | 100 | Số lượng nến cần lấy (1–5000) |

### Biểu đồ nến (MT5-style)

Thêm `-chart` để tạo biểu đồ nến HTML (dùng TradingView Lightweight Charts) và tự mở browser:

```bash
# Demo data + chart
./coinpredict -chart

# Live data + chart
./coinpredict -live -live-snapshots 20 -live-candle 15m -live-interval 30s -chart

# Historical data + chart
./coinpredict -history -history-size 100 -chart

# Chỉ định file output
./coinpredict -chart -chart-out my_chart.html
```

Biểu đồ có dark theme, nến xanh/đỏ, volume, crosshair tooltip, zoom/pan — tương tự MT5.

| Flag | Mặc định | Mô tả |
|------|----------|-------|
| `-chart` | false | Tạo biểu đồ HTML và mở browser |
| `-chart-out` | chart.html | Đường dẫn file HTML output |

### Định dạng dữ liệu

File JSON là một mảng các bản ghi OHLC:

```json
[
  {"date": "2026-11-01", "open": 2835.0, "high": 2850.0, "low": 2828.0, "close": 2845.5},
  {"date": "2026-11-02", "open": 2845.0, "high": 2858.0, "low": 2838.0, "close": 2852.0}
]
```

Xem `sample_data.json` làm ví dụ.

## Kết quả

Chương trình trả về:
- **Hướng** (tăng/giảm/đi ngang)
- **Độ tin cậy** (0–100%)
- **Giá dự đoán** (USD)
- **Mức hỗ trợ & kháng cự**
- **Phân tích** từ LLM

## Cấu trúc project

| File | Mô tả |
|------|-------|
| `main.go` | Điểm vào, xử lý CLI, tải dữ liệu |
| `llm.go` | Gọi API LLM, xây prompt, parse kết quả |
| `price.go` | Gọi API giá vàng realtime (gold-api.com) |
| `chart.go` | Tạo biểu đồ nến HTML (TradingView Lightweight Charts) |
| `sample_data.json` | Dữ liệu giá vàng mẫu |
| `.env.example` | Mẫu cấu hình API |

## Lưu ý

Đây là phân tích do AI tạo ra, **không phải lời khuyên tài chính**. Thị trường vàng chịu nhiều biến động, hãy dùng kết quả làm tham khảo.

```bash
cd /Users/ruby/dev/coin && ./coinpredict -history -history-size 100 -history-interval 15min -chart 2>&1 
```