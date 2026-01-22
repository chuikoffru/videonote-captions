FROM golang:1.25.6-alpine AS builder

WORKDIR /app

# Устанавливаем зависимости для CGO (SQLite)
RUN apk add --no-cache gcc musl-dev

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение с CGO для SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -o bot .

# ---

FROM alpine:latest

# Устанавливаем FFmpeg, шрифты и SQLite
RUN apk add --no-cache \
    ffmpeg \
    font-noto \
    font-noto-emoji \
    ca-certificates \
    sqlite \
    tzdata

WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /app/bot /app/bot

# Создаём директории для временных файлов и базы данных
RUN mkdir -p temp data

CMD ["/app/bot"]

