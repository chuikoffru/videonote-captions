package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"telegram-voice-video-bot/internal/audio"
	"telegram-voice-video-bot/internal/avatar"
	"telegram-voice-video-bot/internal/config"
	"telegram-voice-video-bot/internal/transcription"
	"telegram-voice-video-bot/internal/video"

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
	FileID string
	Author Author
}

// PendingInvoices хранилище ожидающих оплаты
var PendingInvoices sync.Map

// Handler содержит зависимости для обработчиков
type Handler struct {
	Config      *config.Config
	Transcriber *transcription.Service
}

// NewHandler создаёт новый обработчик
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		Config:      cfg,
		Transcriber: transcription.New(cfg.OpenAIAPIKey, cfg.Proxy),
	}
}

// StartHandler обрабатывает команду /start
func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "👋 Привет! Отправь мне голосовое сообщение или перешли чужое, и я создам видео с визуализацией аудио, аватаркой и субтитрами!",
	})
	if err != nil {
		log.Printf("Failed to send start message: %v", err)
	}
}

// VoiceHandler обрабатывает голосовые сообщения
func (h *Handler) VoiceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Voice == nil {
		return
	}

	h.handleVoiceMessage(ctx, b, update.Message, update.Message.Voice, false, nil)
}

// PreCheckoutHandler обрабатывает pre_checkout_query
func (h *Handler) PreCheckoutHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.PreCheckoutQuery == nil {
		return
	}

	payload := update.PreCheckoutQuery.InvoicePayload
	log.Printf("PreCheckoutQuery received, payload: %s", payload)

	if _, ok := PendingInvoices.Load(payload); !ok {
		log.Printf("PreCheckoutQuery: payload not found in PendingInvoices")
		b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
			PreCheckoutQueryID: update.PreCheckoutQuery.ID,
			OK:                 false,
			ErrorMessage:       "❌ Заказ устарел или бот был перезагружен. Пожалуйста, отправьте сообщение снова.",
		})
		return
	}

	log.Printf("PreCheckoutQuery: payload found, answering OK")
	b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
}

// SuccessfulPaymentHandler обрабатывает успешный платёж
func (h *Handler) SuccessfulPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Printf("SuccessfulPaymentHandler called")

	if update.Message == nil || update.Message.SuccessfulPayment == nil {
		log.Printf("SuccessfulPaymentHandler: no SuccessfulPayment in message")
		return
	}

	payload := update.Message.SuccessfulPayment.InvoicePayload
	log.Printf("SuccessfulPayment received, payload: %s", payload)

	value, ok := PendingInvoices.Load(payload)
	if !ok {
		log.Printf("SuccessfulPayment: payload not found in PendingInvoices")
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка: файл не найден (возможно, бот был перезагружен). Перешлите сообщение снова.",
		})
		return
	}

	log.Printf("SuccessfulPayment: payload found, starting video generation")
	orderData := value.(OrderData)
	PendingInvoices.Delete(payload)

	voice := &models.Voice{
		FileID:   orderData.FileID,
		Duration: 0,
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Оплата прошла успешно! Начинаю генерацию видео.",
	})

	h.handleVoiceMessage(ctx, b, update.Message, voice, true, &orderData.Author)
}

// handleVoiceMessage обрабатывает голосовое сообщение
func (h *Handler) handleVoiceMessage(ctx context.Context, b *bot.Bot, msg *models.Message, voice *models.Voice, isPaid bool, authorOverride *Author) {
	// Определяем автора голосового сообщения
	author := authorOverride
	if author == nil {
		author = h.resolveVoiceAuthor(msg)
	}

	// Проверяем длительность и оплату
	if !isPaid && voice.Duration > h.Config.FreeSeconds {
		duration := voice.Duration
		// Расчёт: количество 60-секундных блоков сверх бесплатного лимита
		paidMinutes := (duration - h.Config.FreeSeconds + 59) / 60 // Округление вверх
		price := paidMinutes * h.Config.StarsPerMinute

		payloadID := fmt.Sprintf("%d_%d", msg.From.ID, time.Now().UnixNano())

		log.Printf("Creating invoice with payloadID: %s, fileID: %s", payloadID, voice.FileID)

		PendingInvoices.Store(payloadID, OrderData{
			FileID: voice.FileID,
			Author: *author,
		})

		b.SendInvoice(ctx, &bot.SendInvoiceParams{
			ChatID:      msg.Chat.ID,
			Title:       "Длинное голосовое сообщение",
			Description: fmt.Sprintf("Первые %d секунд — бесплатно! Каждые следующие 60 секунд — %d звёзд. Стоимость твоего видео %d Telegram Stars", h.Config.FreeSeconds, h.Config.StarsPerMinute, price),
			Payload:     payloadID,
			Currency:    "XTR",
			Prices: []models.LabeledPrice{
				{Label: "Видео", Amount: int(price)},
			},
		})
		return
	}

	// Отправляем статусное сообщение
	statusMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "⏳ Обрабатываю голосовое сообщение...",
	})
	if err != nil {
		log.Printf("Failed to send status message: %v", err)
		return
	}

	// Скачиваем аудио
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "⬇️ Загружаю аудио...")
	audioPath, err := audio.DownloadAudio(ctx, b, voice.FileID, h.Config.BotToken)
	if err != nil {
		log.Printf("Failed to download audio: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при обработке. Попробуйте ещё раз.")
		return
	}
	defer os.Remove(audioPath)

	// Транскрибируем
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "📝 Создаю субтитры...")
	transcriptionResult, err := h.Transcriber.TranscribeAudio(ctx, audioPath)
	if err != nil {
		log.Printf("Failed to transcribe: %v", err)
		transcriptionResult = &transcription.Result{Text: "", Words: nil}
	}

	// Скачиваем аватар
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "🖼️ Загружаю аватар...")
	avatarPath, err := avatar.Download(ctx, b, author.ID, author.Type, h.Config.BotToken)
	if err != nil {
		log.Printf("Failed to download avatar: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при обработке. Попробуйте ещё раз.")
		return
	}
	defer os.Remove(avatarPath)

	// Генерируем видео
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "🎬 Создаю видео: 0%")

	lastUpdatePercent := 0
	log.Printf("Video Target: %s", author.Username)

	videoPath, err := video.GenerateVideo(
		audioPath,
		avatarPath,
		transcriptionResult,
		author.Username,
		h.Config.VideoWidth,
		h.Config.VideoHeight,
		h.Config.VideoFPS,
		h.Config.VideoBitrate,
		func(percent int) {
			if percent-lastUpdatePercent >= 5 || percent == 100 {
				lastUpdatePercent = percent
				h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, fmt.Sprintf("🎬 Создаю видео: %d%%", percent))
			}
		},
	)
	if err != nil {
		log.Printf("Failed to generate video: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при обработке. Попробуйте ещё раз.")
		return
	}
	defer os.Remove(videoPath)

	// Отправляем видео
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "📤 Отправляю видео...")

	videoFile, err := os.Open(videoPath)
	if err != nil {
		log.Printf("Failed to open video file: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при обработке. Попробуйте ещё раз.")
		return
	}
	defer videoFile.Close()

	_, err = b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: msg.Chat.ID,
		Video:  &models.InputFileUpload{Filename: "video.mp4", Data: videoFile},
	})
	if err != nil {
		log.Printf("Failed to send video: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при отправке видео.")
		return
	}

	// Удаляем статусное сообщение
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: statusMsg.ID,
	})
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
