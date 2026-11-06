# Deploy CoinPredict lên Render.com

## Tổng quan

CoinPredict chạy chế độ `-watch` liên tục (không phải web server) → deploy dưới dạng **Background Worker** trên Render.

> ⚠️ **Chi phí:** Background Worker **không có free tier**. Gói thấp nhất là **Starter — $7/tháng**.
> Web Service có free tier nhưng không phù hợp vì app không serve HTTP.

---

## Yêu cầu trước khi deploy

- Tài khoản [Render.com](https://dashboard.render.com)
- Repo GitHub/GitLab chứa code (Render pull code từ Git)
- Đủ 4 API keys: `LLM_API_KEY`, `TWELVEDATA_API_KEY`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`

---

## Bước 1 — Push code lên GitHub

```bash
cd /path/to/coin

# Khởi tạo git nếu chưa có
git init
git add .
git commit -m "Initial commit"

# Tạo repo mới trên github.com rồi push
git remote add origin https://github.com/YOUR_USERNAME/coinpredict.git
git branch -M main
git push -u origin main
```

**Quan trọng:** File `.env` đã có trong `.gitignore` — sẽ không bị push lên Git. API keys sẽ được điền trực tiếp trên Render Dashboard.

---

## Bước 2 — Tạo Background Worker trên Render

1. Đăng nhập [dashboard.render.com](https://dashboard.render.com)
2. Click **New +** → chọn **Background Worker**
3. Chọn **Connect a repository** → chọn repo `coinpredict` vừa push
4. Điền thông tin:

| Trường | Giá trị |
|---|---|
| **Name** | `coinpredict-watch` |
| **Region** | `Singapore` (gần VN nhất) |
| **Runtime** | `Docker` (tự detect từ `Dockerfile`) |
| **Instance Type** | `Starter` ($7/tháng) |

5. Click **Advanced** → bỏ qua phần Build & Start Command (Dockerfile tự xử lý)

---

## Bước 3 — Điền Environment Variables

Trong mục **Environment Variables** khi tạo service, thêm từng biến:

| Key | Value |
|---|---|
| `LLM_API_KEY` | API key OpenRouter của bạn (sk-or-...) |
| `LLM_BASE_URL` | `https://openrouter.ai/api/v1` |
| `LLM_MODEL` | `nvidia/nemotron-3-ultra-550b-a55b:free` |
| `TWELVEDATA_API_KEY` | API key Twelve Data |
| `TELEGRAM_BOT_TOKEN` | Token bot Telegram (9033665456:AAG...) |
| `TELEGRAM_CHAT_ID` | Chat ID của bạn (1385834009) |

> **Bảo mật:** Render mã hoá tất cả env vars, không hiển thị lại sau khi save.

---

## Bước 4 — Deploy

1. Click **Create Background Worker**
2. Render tự động:
   - Pull code từ GitHub
   - Build Docker image (Go compile ~1-2 phút)
   - Chạy container với lệnh trong `Dockerfile`
3. Kiểm tra **Logs** tab — bạn sẽ thấy output như:

```
=== WATCH MODE: phân tích mỗi 3m0s, gửi Telegram khi tin cậy ≥ 75% ===
    Interval nến: 15min | Số nến: 50

[14:00:00] Đang phân tích...
Live candle (forming): 14:00  O:4045.10 H:4046.20 ...
...
✅ Đã gửi Telegram lúc 14:00:35
```

---

## Bước 5 — Tuỳ chỉnh lệnh chạy (nếu cần)

Mặc định `Dockerfile` chạy:
```
./coinpredict -watch -history-size 50 -history-interval 15min -watch-interval 3m -min-confidence 0.75
```

Để thay đổi, vào **Render Dashboard → Service → Settings → Start Command**, ghi đè lệnh mới, ví dụ:

```bash
# Mỗi 5 phút, ngưỡng 80%, 100 nến
./coinpredict -watch -history-size 100 -history-interval 15min -watch-interval 5m -min-confidence 0.8
```

Sau khi thay đổi, click **Save Changes** → Render tự restart service.

---

## Tự động redeploy khi push code

Render mặc định bật **Auto-Deploy** — mỗi lần bạn `git push`, Render tự build và deploy lại.

Để tắt: **Settings → Auto-Deploy → Disabled**.

---

## Theo dõi và debug

**Xem logs real-time:**
- Vào **Dashboard → coinpredict-watch → Logs**
- Logs hiển thị từng lần phân tích, kết quả, và Telegram status

**Restart thủ công:**
- **Dashboard → coinpredict-watch → Manual Deploy → Deploy latest commit**

**Dừng service:**
- **Dashboard → coinpredict-watch → Suspend**
- Service bị suspend sẽ không tính tiền

---

## Cấu trúc file đã thêm

| File | Mô tả |
|---|---|
| `Dockerfile` | Build Go binary trong Alpine Linux, chạy watch mode |
| `render.yaml` | Infrastructure-as-code: định nghĩa service, region, env vars |

---

## Chi phí ước tính

| Plan | CPU | RAM | Giá |
|---|---|---|---|
| Starter | 0.5 CPU | 512 MB | **$7/tháng** |
| Standard | 1 CPU | 2 GB | $25/tháng |

Starter là đủ cho CoinPredict — app không tốn nhiều CPU/RAM, chủ yếu chờ giữa các lần phân tích.

---

## Lưu ý quan trọng

- **Twelve Data free**: 800 credits/ngày. Với `-watch-interval 3m`, app gọi ~480 lần/ngày — trong giới hạn.
- **OpenRouter free model**: Có thể bị rate limit lúc nhiều người dùng. Nếu hay lỗi, cân nhắc dùng Groq (có free tier nhanh hơn).
- **gold-api.com**: Không giới hạn request — dùng cho nến live và `-live` mode.
- Render có thể **restart service bất cứ lúc nào** — app sẽ tự khởi động lại và tiếp tục phân tích.
