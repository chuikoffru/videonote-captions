FROM golang:1.22-bullseye AS builder

WORKDIR /app

# Устанавливаем зависимости для CGO (SQLite)
RUN apt-get update && apt-get install -y gcc libc6-dev

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение с CGO для SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -o bot .

# ---

FROM debian:bullseye-slim

# Устанавливаем FFmpeg, шрифты и SQLite
RUN apt-get update && apt-get install -y \
    ffmpeg \
    fonts-liberation \
    fonts-noto \
    fonts-noto-color-emoji \
    ca-certificates \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /app/bot /app/bot

# Создаём директории для временных файлов и базы данных
RUN mkdir -p temp data

CMD ["/app/bot"]

