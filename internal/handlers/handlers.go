package handlers

import (
	"context"
	"github.com/go-telegram/bot"
	"videonote-captions/internal/audio"
	"videonote-captions/internal/config"
	"videonote-captions/internal/database"
	"videonote-captions/internal/transcription"
	"videonote-captions/internal/video"
)

type Handler struct {
	Config      *config.Config
	Transcriber interface {
		TranscribeAudio(context.Context, string) (*transcription.Result, error)
	}
	DB       *database.DB
	slots    chan struct{}
	pending  *pendingVideoStore
	billing  conversionBilling
	media    mediaTools
	tempRoot string
}

func NewHandler(cfg *config.Config, db *database.DB) *Handler {
	copyConfig := *cfg
	if copyConfig.PlainCirclesPerDay <= 0 {
		copyConfig.PlainCirclesPerDay = 5
	}
	if copyConfig.MaxInputVideoBytes <= 0 || copyConfig.MaxInputVideoBytes > 20971520 {
		copyConfig.MaxInputVideoBytes = 20971520
	}
	if copyConfig.MaxInputVideoSeconds <= 0 {
		copyConfig.MaxInputVideoSeconds = 600
	}
	if copyConfig.VideoProcessTimeoutSeconds <= 0 {
		copyConfig.VideoProcessTimeoutSeconds = 180
	}
	h := &Handler{Config: &copyConfig, Transcriber: transcription.New(cfg.OpenAIAPIKey, cfg.Proxy), DB: db, slots: make(chan struct{}, 2), pending: newPendingVideoStore(), tempRoot: "temp"}
	if db != nil {
		h.billing = db
	}
	h.media = mediaTools{Download: audio.DownloadTelegramVideo, Probe: video.Probe, Convert: video.ConvertToCircle, Extract: audio.ExtractAudioFromVideoContext, Caption: video.AddSubtitlesToVideoNoteContext}
	return h
}
func (h *Handler) updateStatus(ctx context.Context, b *bot.Bot, chatID int64, messageID int, text string) {
	b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text})
}
