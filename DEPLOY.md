# Deploy CoinPredict — Railway & Oracle Cloud

---

## Lựa chọn nào phù hợp với bạn?

| | Railway | Oracle Cloud |
|---|---|---|
| **Chi phí** | Free 30 ngày ($5 credit), sau đó ~$1-2/tháng (Hobby $5/tháng) | **Miễn phí vĩnh viễn** |
| **Độ khó setup** | ⭐ Rất dễ (5 phút) | ⭐⭐⭐ Trung bình (~30 phút) |
| **Yêu cầu** | GitHub account | Thẻ tín dụng (verify, không bị tính tiền) |
| **Phù hợp** | Muốn deploy nhanh, chấp nhận $1-2/tháng | Muốn hoàn toàn miễn phí |

---

# PHẦN 1 — Railway

## Chi phí thực tế

- **Hobby plan**: $5/tháng, bao gồm $5 credit dùng thật
- CoinPredict với `-watch -watch-interval 3m` dùng khoảng 0.1 vCPU × 720 giờ/tháng
- Ước tính: **~$1-2/tháng** — nằm trong $5 credit → **thực chất không phát sinh thêm**

## Bước 1 — Chuẩn bị

Code đã có `Dockerfile` sẵn. Chỉ cần push lên GitHub:

```bash
cd /path/to/coin
git add Dockerfile render.yaml   # thêm nếu chưa commit
git commit -m "Add Dockerfile"
git push
```

## Bước 2 — Tạo tài khoản Railway

1. Vào [railway.com](https://railway.com) → **Login with GitHub**
2. Xác nhận quyền truy cập GitHub

## Bước 3 — Tạo project

1. Click **New Project**
2. Chọn **Deploy from GitHub repo**
3. Chọn repo `coinpredict`
4. Railway tự detect `Dockerfile` → click **Deploy Now**

## Bước 4 — Điền Environment Variables

1. Vào service vừa tạo → tab **Variables**
2. Click **New Variable**, thêm lần lượt:

```
LLM_API_KEY          = sk-or-...
LLM_BASE_URL         = https://openrouter.ai/api/v1
LLM_MODEL            = nvidia/nemotron-3-ultra-550b-a55b:free
TWELVEDATA_API_KEY   = 61a2855c25e049c8b6c06228b1a1825b
TELEGRAM_BOT_TOKEN   = 9033665456:AAGjrn8T3pNaAI-yAY9IckbZWtcRLtuakrU
TELEGRAM_CHAT_ID     = 1385834009
```

3. Railway tự restart service sau khi thêm biến

## Bước 5 — Kiểm tra

- Tab **Deployments** → xem build log
- Tab **Logs** → thấy output watch mode là thành công:

```
=== WATCH MODE: phân tích mỗi 3m0s, gửi Telegram khi tin cậy ≥ 75% ===
[14:00:00] Đang phân tích...
✅ Đã gửi Telegram lúc 14:00:35
```

## Tuỳ chỉnh lệnh chạy

Vào **Settings → Deploy → Custom Start Command**:
```bash
./coinpredict -watch -history-size 50 -history-interval 15min -watch-interval 5m -min-confidence 0.8
```

## Nâng cấp lên Hobby plan

Free trial giới hạn 30 ngày. Để chạy lâu dài:
1. **Account → Billing → Upgrade to Hobby** ($5/tháng)
2. Service tiếp tục chạy không gián đoạn

---

# PHẦN 2 — Oracle Cloud Always Free

## Tại sao Oracle Cloud?

Oracle cung cấp **Always Free** (vĩnh viễn, không hết hạn):
- 2 AMD micro VM (1 vCPU, 1 GB RAM mỗi cái)
- **hoặc** tổng 4 ARM vCPU + 24 GB RAM (linh hoạt chia)
- 200 GB block storage
- Cần thẻ tín dụng để verify (không bị charge nếu trong giới hạn free)

## Bước 1 — Tạo tài khoản Oracle Cloud

1. Vào [cloud.oracle.com](https://cloud.oracle.com) → **Start for free**
2. Điền thông tin, verify email
3. Nhập thẻ tín dụng (bắt buộc để verify, không bị charge)
4. Chọn **Home Region**: **Japan East (Tokyo)** hoặc **Singapore** — gần VN, latency thấp

> ⚠️ Chọn Home Region **không thể thay đổi sau này**

## Bước 2 — Tạo VM Instance

1. Vào **Compute → Instances → Create Instance**
2. Cấu hình:

| Trường | Giá trị |
|---|---|
| **Name** | `coinpredict-worker` |
| **Image** | Ubuntu 22.04 (Minimal) |
| **Shape** | `VM.Standard.A1.Flex` (ARM — Always Free) |
| **OCPUs** | 1 |
| **Memory** | 6 GB |
| **Boot Volume** | 50 GB |

3. **Add SSH keys** → Upload file `~/.ssh/id_rsa.pub` hoặc tạo key mới
4. Click **Create**

> Nếu báo "Out of capacity" với ARM, thử lại sau vài giờ hoặc dùng `VM.Standard.E2.1.Micro` (AMD, 1 vCPU, 1 GB RAM — cũng Always Free)

## Bước 3 — Kết nối SSH vào VM

```bash
# Lấy Public IP từ Oracle Dashboard → Instance Details
ssh ubuntu@<PUBLIC_IP>

# Ví dụ:
ssh ubuntu@140.238.xx.xx
```

## Bước 4 — Cài đặt môi trường

```bash
# Cập nhật hệ thống
sudo apt update && sudo apt upgrade -y

# Cài Go 1.22
wget https://go.dev/dl/go1.22.0.linux-arm64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-arm64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Kiểm tra
go version
```

> Nếu dùng AMD (x86), thay `linux-arm64` bằng `linux-amd64`

## Bước 5 — Upload và build code

**Cách 1 — Clone từ GitHub (khuyên dùng):**
```bash
git clone https://github.com/YOUR_USERNAME/coinpredict.git
cd coinpredict
go build -o coinpredict .
```

**Cách 2 — Upload trực tiếp bằng scp:**
```bash
# Chạy từ máy local
scp -r /Users/ruby/dev/coin ubuntu@<PUBLIC_IP>:~/coinpredict
```

## Bước 6 — Tạo file .env trên server

```bash
cd ~/coinpredict
nano .env
```

Dán nội dung:
```env
LLM_API_KEY=sk-or-...
LLM_BASE_URL=https://openrouter.ai/api/v1
LLM_MODEL=nvidia/nemotron-3-ultra-550b-a55b:free
TWELVEDATA_API_KEY=61a2855c25e049c8b6c06228b1a1825b
TELEGRAM_BOT_TOKEN=9033665456:AAGjrn8T3pNaAI-yAY9IckbZWtcRLtuakrU
TELEGRAM_CHAT_ID=1385834009
```

Lưu: `Ctrl+X` → `Y` → `Enter`

```bash
# Bảo vệ file .env
chmod 600 .env
```

## Bước 7 — Chạy background bằng systemd

Tạo service để tự restart khi server reboot:

```bash
sudo nano /etc/systemd/system/coinpredict.service
```

Dán nội dung:
```ini
[Unit]
Description=CoinPredict Watch Service
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/coinpredict
ExecStart=/home/ubuntu/coinpredict/coinpredict -watch -history-size 50 -history-interval 15min -watch-interval 3m -min-confidence 0.75
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Lưu và kích hoạt:
```bash
sudo systemctl daemon-reload
sudo systemctl enable coinpredict    # tự start khi reboot
sudo systemctl start coinpredict
sudo systemctl status coinpredict    # kiểm tra đang chạy
```

## Bước 8 — Xem logs

```bash
# Xem logs real-time
journalctl -u coinpredict -f

# Xem 50 dòng gần nhất
journalctl -u coinpredict -n 50
```

Output mong đợi:
```
Nov 06 14:00:00 coinpredict[1234]: === WATCH MODE: phân tích mỗi 3m0s ...
Nov 06 14:00:35 coinpredict[1234]: ✅ Đã gửi Telegram lúc 14:00:35
```

## Quản lý service

```bash
sudo systemctl stop coinpredict      # dừng
sudo systemctl restart coinpredict   # restart
sudo systemctl disable coinpredict   # tắt auto-start
```

## Cập nhật code

```bash
cd ~/coinpredict
git pull                            # nếu dùng GitHub
go build -o coinpredict .
sudo systemctl restart coinpredict
```

---

# So sánh tổng kết

| | Railway | Oracle Cloud |
|---|---|---|
| **Setup time** | ~5 phút | ~30 phút |
| **Chi phí** | ~$1-2/tháng (trong $5 Hobby credit) | **$0 mãi mãi** |
| **Logs** | Dashboard web đẹp | `journalctl` CLI |
| **Cập nhật code** | `git push` tự động | `git pull` + rebuild thủ công |
| **Uptime** | 99.9% SLA | Tốt, nhưng không có SLA |
| **Khó khăn** | Không có | Setup lần đầu phức tạp hơn |
| **Phù hợp** | Muốn tiện lợi, chấp nhận trả nhỏ | Muốn $0 hoàn toàn |
