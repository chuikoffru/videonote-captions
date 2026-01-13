FROM golang:1.22-bullseye AS builder

WORKDIR /app

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -o bot .

# ---

FROM debian:bullseye-slim

# Устанавливаем FFmpeg и шрифты
RUN apt-get update && apt-get install -y \
    ffmpeg \
    fonts-liberation \
    fonts-noto \
    fonts-noto-color-emoji \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /app/bot /app/bot

# Создаём директорию для временных файлов
RUN mkdir -p temp

CMD ["/app/bot"]
