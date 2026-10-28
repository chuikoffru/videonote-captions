package handlers

import (
	"context"
	"github.com/go-telegram/bot"
	"videonote-captions/internal/config"
	"videonote-captions/internal/database"
	"videonote-captions/internal/transcription"
)

type Handler struct {
	Config      *config.Config
	Transcriber *transcription.Service
	DB          *database.DB
	slots       chan struct{}
}

func NewHandler(cfg *config.Config, db *database.DB) *Handler {
	return &Handler{Config: cfg, Transcriber: transcription.New(cfg.OpenAIAPIKey, cfg.Proxy), DB: db, slots: make(chan struct{}, 2)}
}
func (h *Handler) updateStatus(ctx context.Context, b *bot.Bot, chatID int64, messageID int, text string) {
	b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text})
}
