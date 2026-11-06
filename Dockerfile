# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY *.go .
RUN go build -o coinpredict .

# Run stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/coinpredict .

# Default: watch mode, 50 candles 15min, send Telegram khi confidence >= 75%, mỗi 3 phút
CMD ["./coinpredict", "-watch", "-history-size", "50", "-history-interval", "15min", "-watch-interval", "3m", "-min-confidence", "0.75"]
