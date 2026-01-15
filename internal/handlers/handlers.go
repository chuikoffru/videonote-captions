package handlers

import (
	"context"
	"strings"
	"sync"

	"telegram-voice-video-bot/internal/config"
	"telegram-voice-video-bot/internal/database"
	"telegram-voice-video-bot/internal/transcription"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Author представляет автора голосового сообщения
type Author struct {
	ID       int64
	Username string
	Type     string // "user", "chat", "channel", "hidden_user"
}

// OrderData хранит данные заказа для оплаты
type OrderData struct {
	Type                string // "voice" or "vertical"
	FileID              string // Для голоса - file_id голосового, для вертикального - file_id голосового
	FileUniqueID        string // file_unique_id для кэширования
	OriginalVideoFileID string // Только для вертикального видео
	Author              Author
}

// PendingInvoices хранилище ожидающих оплаты
var PendingInvoices sync.Map

// VerticalRequests хранилище данных для запросов вертикального видео (key -> OrderData)
var VerticalRequests sync.Map
var verticalRequestCounter int64

// Handler содержит зависимости для обработчиков
type Handler struct {
	Config      *config.Config
	Transcriber *transcription.Service
	DB          *database.DB
}

// NewHandler создаёт новый обработчик
func NewHandler(cfg *config.Config, db *database.DB) *Handler {
	return &Handler{
		Config:      cfg,
		Transcriber: transcription.New(cfg.OpenAIAPIKey, cfg.Proxy),
		DB:          db,
	}
}

// updateStatus обновляет статусное сообщение
func (h *Handler) updateStatus(ctx context.Context, b *bot.Bot, chatID int64, messageID int, text string) {
	b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
}

// resolveVoiceAuthor определяет автора голосового сообщения
func (h *Handler) resolveVoiceAuthor(msg *models.Message) *Author {
	// Проверяем forward_origin (Bot API 7.0+)
	if msg.ForwardOrigin != nil {
		switch msg.ForwardOrigin.Type {
		case models.MessageOriginTypeUser:
			if msg.ForwardOrigin.MessageOriginUser != nil {
				user := msg.ForwardOrigin.MessageOriginUser.SenderUser
				return &Author{
					ID:       user.ID,
					Username: h.getUserDisplayName(&user),
					Type:     "user",
				}
			}
		case models.MessageOriginTypeHiddenUser:
			if msg.ForwardOrigin.MessageOriginHiddenUser != nil {
				return &Author{
					ID:       0,
					Username: msg.ForwardOrigin.MessageOriginHiddenUser.SenderUserName,
					Type:     "hidden_user",
				}
			}
		case models.MessageOriginTypeChat:
			if msg.ForwardOrigin.MessageOriginChat != nil {
				chat := msg.ForwardOrigin.MessageOriginChat.SenderChat
				return &Author{
					ID:       chat.ID,
					Username: chat.Title,
					Type:     "chat",
				}
			}
		case models.MessageOriginTypeChannel:
			if msg.ForwardOrigin.MessageOriginChannel != nil {
				chat := msg.ForwardOrigin.MessageOriginChannel.Chat
				return &Author{
					ID:       chat.ID,
					Username: chat.Title,
					Type:     "channel",
				}
			}
		}
	}

	// Если не переслано — берём текущего отправителя
	return &Author{
		ID:       msg.From.ID,
		Username: h.getUserDisplayName(msg.From),
		Type:     "user",
	}
}

// getUserDisplayName возвращает отображаемое имя пользователя
func (h *Handler) getUserDisplayName(user *models.User) string {
	if user.Username != "" {
		return user.Username
	}
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	return strings.TrimSpace(name)
}
