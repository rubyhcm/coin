# CoinPredict — Dự đoán giá Gold/USD bằng LLM

Ứng dụng CLI Go lấy dữ liệu nến OHLC XAU/USD, vẽ biểu đồ MT5-style, dùng LLM để dự đoán xu hướng + đề xuất trade setup (entry / SL / TP), và tự động gửi tín hiệu qua Telegram.

## Yêu cầu

- Go 1.21+
- API key LLM (OpenAI, Groq, OpenRouter, Ollama, v.v.)
- API key Twelve Data — miễn phí tại https://twelvedata.com/pricing
- Telegram Bot Token + Chat ID — miễn phí (chỉ cần cho `-watch`)

## Cài đặt

```bash
cp .env.example .env   # tạo file cấu hình
# điền LLM_API_KEY, TWELVEDATA_API_KEY, TELEGRAM_* vào .env
go build -o coinpredict .
```

## Sử dụng nhanh

```bash
# Lịch sử 100 nến 15 phút + biểu đồ + dự đoán + trade setup
./coinpredict -history -history-size 100 -history-interval 15min -chart

# Chỉ dự đoán, không vẽ biểu đồ
./coinpredict -history

# Tự động phân tích mỗi 3 phút, gửi Telegram khi tin cậy ≥ 75%
./coinpredict -watch -history-size 50 -history-interval 15min

# Khung 1 giờ
./coinpredict -history -history-size 100 -history-interval 1h -chart
```

## Tài liệu

| File | Nội dung |
|---|---|
| `USAGE.md` | Hướng dẫn sử dụng đầy đủ, tất cả flag, ví dụ lệnh |
| `TECHNICAL.md` | Tài liệu kỹ thuật: kiến trúc, hàm, luồng xử lý |

## Cấu trúc project

| File | Mô tả |
|---|---|
| `main.go` | CLI, routing chế độ, orchestration |
| `price.go` | Lấy dữ liệu giá (gold-api.com, Twelve Data) |
| `llm.go` | Gọi LLM API, build prompt, parse kết quả |
| `chart.go` | Tạo biểu đồ nến HTML (MT5-style) |
| `telegram.go` | Gửi tín hiệu qua Telegram Bot API |
| `sample_data.json` | Dữ liệu mẫu để test với `-data` |

> Đây là phân tích do AI tạo ra, **không phải lời khuyên tài chính**.
