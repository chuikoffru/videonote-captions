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
	"telegram-voice-video-bot/internal/database"
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

// StartHandler обрабатывает команду /start
func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	// Регистрируем пользователя
	if h.DB != nil && update.Message.From != nil {
		_, err := h.DB.GetOrCreateUser(
			update.Message.From.ID,
			update.Message.From.Username,
			update.Message.From.FirstName,
			update.Message.From.LastName,
		)
		if err != nil {
			log.Printf("Failed to register user: %v", err)
		}
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "👋 Привет! Отправь мне голосовое сообщение или перешли чужое, и я создам видео с визуализацией аудио, аватаркой и субтитрами!",
	})
	if err != nil {
		log.Printf("Failed to send start message: %v", err)
	}
}

// StatsHandler обрабатывает команду /stats (только для админа)
func (h *Handler) StatsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	// Проверяем, что это админ
	if update.Message.From.ID != h.Config.AdminID {
		log.Printf("Stats requested by non-admin user: %d", update.Message.From.ID)
		return
	}

	if h.DB == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ База данных не инициализирована.",
		})
		return
	}

	stats, userCount, err := h.DB.GetStats()
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка получения статистики.",
		})
		return
	}

	statsText := fmt.Sprintf(`📊 *Статистика бота*

👥 Пользователей: %d
🎬 Бесплатных видео: %d
💳 Запросов оплаты: %d
✅ Оплаченных видео: %d`,
		userCount,
		stats.TotalFreeVideos,
		stats.TotalPaymentRequests,
		stats.TotalPaidVideos,
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      statsText,
		ParseMode: models.ParseModeMarkdown,
	})
}

// VoiceHandler обрабатывает голосовые сообщения
func (h *Handler) VoiceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Voice == nil {
		return
	}

	h.handleVoiceMessage(ctx, b, update.Message, update.Message.Voice, false, nil, update.Message.Voice.FileUniqueID)
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
			Text:   "❌ Ошибка: заказ не найден (возможно, бот был перезагружен). Повторите попытку.",
		})
		return
	}

	log.Printf("SuccessfulPayment: payload found, starting processing")
	orderData := value.(OrderData)
	PendingInvoices.Delete(payload)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Оплата прошла успешно! Начинаю обработку.",
	})

	if orderData.Type == "vertical" {
		h.handleVerticalVideoRequest(ctx, b, update.Message, orderData.FileID, orderData.FileUniqueID, &orderData.Author)
	} else {
		voice := &models.Voice{
			FileID:       orderData.FileID,
			FileUniqueID: orderData.FileUniqueID,
			Duration:     0,
		}
		h.handleVoiceMessage(ctx, b, update.Message, voice, true, &orderData.Author, orderData.FileUniqueID)
	}
}

// VerticalCallbackHandler обрабатывает нажатие на кнопку "Сделать вертикальное видео"
func (h *Handler) VerticalCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Printf("VerticalCallbackHandler called")

	if update.CallbackQuery == nil {
		log.Printf("CallbackQuery is nil")
		return
	}

	log.Printf("CallbackQuery data: %s", update.CallbackQuery.Data)

	data := update.CallbackQuery.Data
	if !strings.HasPrefix(data, "vert:") {
		log.Printf("Data does not start with 'vert:': %s", data)
		return
	}

	// Извлекаем ключ из callback_data
	vertKey := strings.TrimPrefix(data, "vert:")

	// Получаем данные из хранилища
	value, ok := VerticalRequests.Load(vertKey)
	if !ok {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "❌ Запрос устарел. Отправьте голосовое сообщение снова.",
			ShowAlert:       true,
		})
		return
	}
	log.Printf("Found vertical request with key: %s", vertKey)
	orderData := value.(OrderData)

	// Получаем chat_id из callback query
	chatID := update.CallbackQuery.From.ID
	if update.CallbackQuery.Message.Message != nil {
		chatID = update.CallbackQuery.Message.Message.Chat.ID
	}

	// Проверяем кэш вертикального видео — если есть, отправляем бесплатно
	if h.DB != nil && orderData.FileUniqueID != "" {
		cache, err := h.DB.GetCachedVideo(orderData.FileUniqueID)
		if err != nil {
			log.Printf("Ошибка проверки кэша: %v", err)
		} else if cache != nil && cache.VerticalVideoFileID != "" {
			log.Printf("✅ Найден кэш вертикального видео, отправляем бесплатно")

			// Отвечаем на callback
			b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            "✅ Видео найдено в кэше!",
			})

			// Отправляем видео из кэша
			_, err := b.SendVideo(ctx, &bot.SendVideoParams{
				ChatID: chatID,
				Video:  &models.InputFileString{Data: cache.VerticalVideoFileID},
			})
			if err != nil {
				log.Printf("Ошибка отправки вертикального видео из кэша: %v", err)
				b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   "❌ Ошибка отправки видео. Попробуйте ещё раз.",
				})
			}
			return
		}
	}

	// Отвечаем на callback чтобы убрать часики
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	// Создаем счет на оплату
	payloadID := fmt.Sprintf("vert_%d_%d", update.CallbackQuery.From.ID, time.Now().UnixNano())

	PendingInvoices.Store(payloadID, orderData)

	// Логируем запрос оплаты
	if h.DB != nil {
		h.DB.LogPaymentRequest()
	}

	log.Printf("Sending invoice to chat %d", chatID)
	b.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:      chatID,
		Title:       "Вертикальное видео (9:16)",
		Description: "Видео будет адаптировано под формат TikTok/Reels/Shorts.",
		Payload:     payloadID,
		Currency:    "XTR",
		Prices: []models.LabeledPrice{
			{Label: "Вертикальное видео", Amount: h.Config.VerticalCost},
		},
	})
}

// handleVoiceMessage обрабатывает голосовое сообщение
func (h *Handler) handleVoiceMessage(ctx context.Context, b *bot.Bot, msg *models.Message, voice *models.Voice, isPaid bool, authorOverride *Author, voiceFileUniqueID string) {
	// Определяем автора голосового сообщения
	author := authorOverride
	if author == nil {
		author = h.resolveVoiceAuthor(msg)
	}

	// Регистрируем пользователя если ещё не зарегистрирован
	if h.DB != nil && msg.From != nil {
		h.DB.GetOrCreateUser(
			msg.From.ID,
			msg.From.Username,
			msg.From.FirstName,
			msg.From.LastName,
		)
	}

	// Проверяем кэш перед обработкой
	if h.DB != nil && voiceFileUniqueID != "" {
		cache, err := h.DB.GetCachedVideo(voiceFileUniqueID)
		if err != nil {
			log.Printf("Ошибка проверки кэша: %v", err)
		} else if cache != nil && cache.VideoFileID != "" {
			log.Printf("✅ Найден кэш для voice_file_unique_id=%s, отправляем видео из кэша", voiceFileUniqueID)

			// Создаем короткий ключ для callback_data
			verticalRequestCounter++
			vertKey := fmt.Sprintf("v%d", verticalRequestCounter)
			VerticalRequests.Store(vertKey, OrderData{
				Type:         "vertical",
				FileID:       voice.FileID,
				FileUniqueID: voiceFileUniqueID,
				Author:       *author,
			})

			// Отправляем видео из кэша по file_id
			_, err := b.SendVideo(ctx, &bot.SendVideoParams{
				ChatID: msg.Chat.ID,
				Video:  &models.InputFileString{Data: cache.VideoFileID},
				ReplyMarkup: &models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{
						{
							{
								Text:         fmt.Sprintf("📱 Сделать вертикальное видео (%d⭐)", h.Config.VerticalCost),
								CallbackData: "vert:" + vertKey,
							},
						},
					},
				},
			})
			if err != nil {
				log.Printf("Ошибка отправки видео из кэша: %v", err)
				// Продолжаем обычную обработку если кэш не сработал
			} else {
				return // Успешно отправили из кэша
			}
		}
	}

	// Проверяем, является ли пользователь админом (админ без ограничений)
	isAdmin := msg.From != nil && msg.From.ID == h.Config.AdminID

	// Проверяем длительность и оплату (админ пропускает оплату)
	if !isPaid && !isAdmin && voice.Duration > h.Config.FreeSeconds {
		duration := voice.Duration
		// Расчёт: количество 60-секундных блоков сверх бесплатного лимита
		paidMinutes := (duration - h.Config.FreeSeconds + 59) / 60 // Округление вверх
		price := paidMinutes * h.Config.StarsPerMinute

		payloadID := fmt.Sprintf("%d_%d", msg.From.ID, time.Now().UnixNano())

		log.Printf("Creating invoice with payloadID: %s, fileID: %s", payloadID, voice.FileID)

		PendingInvoices.Store(payloadID, OrderData{
			Type:         "voice",
			FileID:       voice.FileID,
			FileUniqueID: voiceFileUniqueID,
			Author:       *author,
		})

		// Логируем запрос оплаты
		if h.DB != nil {
			h.DB.LogPaymentRequest()
		}

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

	// Создаем короткий ключ для callback_data (ограничение Telegram - 64 байта)
	verticalRequestCounter++
	vertKey := fmt.Sprintf("v%d", verticalRequestCounter)
	VerticalRequests.Store(vertKey, OrderData{
		Type:   "vertical",
		FileID: voice.FileID,
		Author: *author,
	})

	videoResult, err := b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: msg.Chat.ID,
		Video:  &models.InputFileUpload{Filename: "video.mp4", Data: videoFile},
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text:         fmt.Sprintf("📱 Сделать вертикальное видео (%d⭐)", h.Config.VerticalCost),
						CallbackData: "vert:" + vertKey,
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("Failed to send video: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при отправке видео.")
		return
	}

	// Логируем видео в базу данных
	if h.DB != nil && msg.From != nil {
		videoFileID := ""
		if videoResult.Video != nil {
			videoFileID = videoResult.Video.FileID
		}
		if isPaid {
			h.DB.LogPaidVideo(msg.From.ID, voice.Duration, videoFileID)
		} else {
			h.DB.LogFreeVideo(msg.From.ID, voice.Duration, videoFileID)
		}

		// Сохраняем в кэш
		if voiceFileUniqueID != "" && videoFileID != "" {
			err := h.DB.SaveVideoCache(voiceFileUniqueID, videoFileID)
			if err != nil {
				log.Printf("Ошибка сохранения кэша видео: %v", err)
			} else {
				log.Printf("✅ Сохранён кэш видео для voice_file_unique_id=%s", voiceFileUniqueID)
			}
		}
	}

	// Удаляем статусное сообщение
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: statusMsg.ID,
	})
}

// handleVerticalVideoRequest обрабатывает запрос на создание вертикального видео
func (h *Handler) handleVerticalVideoRequest(ctx context.Context, b *bot.Bot, msg *models.Message, voiceFileID string, voiceFileUniqueID string, author *Author) {
	// Проверяем кэш вертикального видео
	if h.DB != nil && voiceFileUniqueID != "" {
		cache, err := h.DB.GetCachedVideo(voiceFileUniqueID)
		if err != nil {
			log.Printf("Ошибка проверки кэша вертикального видео: %v", err)
		} else if cache != nil && cache.VerticalVideoFileID != "" {
			log.Printf("✅ Найден кэш вертикального видео для voice_file_unique_id=%s", voiceFileUniqueID)

			// Отправляем видео из кэша
			_, err := b.SendVideo(ctx, &bot.SendVideoParams{
				ChatID: msg.Chat.ID,
				Video:  &models.InputFileString{Data: cache.VerticalVideoFileID},
			})
			if err != nil {
				log.Printf("Ошибка отправки вертикального видео из кэша: %v", err)
				// Продолжаем обычную обработку
			} else {
				return // Успешно отправили из кэша
			}
		}
	}

	// Отправляем статусное сообщение
	statusMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "⏳ Начинаю создание вертикального видео...",
	})
	if err != nil {
		log.Printf("Failed to send status message: %v", err)
		return
	}

	// Скачиваем аудио
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "⬇️ Загружаю аудио...")
	audioPath, err := audio.DownloadAudio(ctx, b, voiceFileID, h.Config.BotToken)
	if err != nil {
		log.Printf("Failed to download audio: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Ошибка при загрузке аудио.")
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
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Ошибка при загрузке аватара.")
		return
	}
	defer os.Remove(avatarPath)

	// Генерируем вертикальное видео
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "🎬 Создаю вертикальное видео: 0%")

	lastUpdatePercent := 0
	videoPath, err := video.GenerateVerticalVideo(
		audioPath,
		avatarPath,
		transcriptionResult,
		author.Username,
		h.Config.VideoFPS,
		h.Config.VideoBitrate,
		func(percent int) {
			if percent-lastUpdatePercent >= 5 || percent == 100 {
				lastUpdatePercent = percent
				h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, fmt.Sprintf("🎬 Создаю вертикальное видео: %d%%", percent))
			}
		},
	)
	if err != nil {
		log.Printf("Failed to generate vertical video: %v", err)
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Ошибка при создании видео.")
		return
	}
	defer os.Remove(videoPath)

	// Отправляем видео
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "📤 Отправлю результат...")

	videoFile, err := os.Open(videoPath)
	if err != nil {
		log.Printf("Failed to open video file: %v", err)
		return
	}
	defer videoFile.Close()

	videoResult, err := b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: msg.Chat.ID,
		Video:  &models.InputFileUpload{Filename: "vertical_video.mp4", Data: videoFile},
	})
	if err != nil {
		log.Printf("Failed to send vertical video: %v", err)
	}

	// Сохраняем в кэш
	if h.DB != nil && voiceFileUniqueID != "" && videoResult != nil && videoResult.Video != nil {
		err := h.DB.SaveVerticalVideoCache(voiceFileUniqueID, videoResult.Video.FileID)
		if err != nil {
			log.Printf("Ошибка сохранения кэша вертикального видео: %v", err)
		} else {
			log.Printf("✅ Сохранён кэш вертикального видео для voice_file_unique_id=%s", voiceFileUniqueID)
		}
	}

	// Логируем в БД
	if h.DB != nil {
		h.DB.LogPaidVideo(msg.From.ID, 0, "")
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
