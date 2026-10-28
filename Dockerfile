ARG GO_IMAGE=golang:1.25.6-alpine
ARG RUNTIME_IMAGE=alpine:3.23
FROM ${GO_IMAGE} AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /bot .

FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ffmpeg font-noto font-noto-emoji ca-certificates tzdata && adduser -D bot
WORKDIR /app
COPY --from=builder /bot /app/bot
RUN mkdir temp && chown bot:bot temp
USER bot
CMD ["/app/bot"]
