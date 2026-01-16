package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"telegram-voice-video-bot/internal/audio"
	"telegram-voice-video-bot/internal/avatar"
	"telegram-voice-video-bot/internal/transcription"
	"telegram-voice-video-bot/internal/video"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

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

// VerticalCacheCallbackHandler обрабатывает нажатие на кнопку "Получить вертикальное видео" (из кэша, бесплатно)
func (h *Handler) VerticalCacheCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Printf("VerticalCacheCallbackHandler called")

	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	if !strings.HasPrefix(data, "vertcache:") {
		return
	}

	// Извлекаем ключ из callback_data
	vertKey := strings.TrimPrefix(data, "vertcache:")

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
	orderData := value.(OrderData)

	// Получаем chat_id
	chatID := update.CallbackQuery.From.ID
	if update.CallbackQuery.Message.Message != nil {
		chatID = update.CallbackQuery.Message.Message.Chat.ID
	}

	// Получаем видео из кэша
	if h.DB == nil || orderData.FileUniqueID == "" {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "❌ Ошибка: кэш недоступен.",
			ShowAlert:       true,
		})
		return
	}

	cache, err := h.DB.GetCachedVideo(orderData.FileUniqueID)
	if err != nil || cache == nil || cache.VerticalVideoFileID == "" {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "❌ Видео не найдено в кэше.",
			ShowAlert:       true,
		})
		return
	}

	// Отвечаем на callback
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "✅ Отправляю видео из кэша!",
	})

	// Отправляем видео из кэша
	_, err = b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: chatID,
		Video:  &models.InputFileString{Data: cache.VerticalVideoFileID},
	})
	if err != nil {
		log.Printf("Ошибка отправки вертикального видео из кэша: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка отправки видео.",
		})
	} else {
		log.Printf("✅ Вертикальное видео отправлено из кэша")
	}
}

// handleVerticalVideoRequest обрабатывает запрос на создание вертикального видео
func (h *Handler) handleVerticalVideoRequest(ctx context.Context, b *bot.Bot, msg *models.Message, voiceFileID string, voiceFileUniqueID string, author *Author) {
	log.Printf("Начинаем генерацию вертикального видео для voice_file_unique_id=%s", voiceFileUniqueID)

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

	// Проверяем кэш транскрипции
	var cachedTranscription *transcription.Result
	if h.DB != nil && voiceFileUniqueID != "" {
		cache, _ := h.DB.GetCachedVideo(voiceFileUniqueID)
		if cache != nil && cache.TranscriptionJSON != "" {
			cachedTranscription, _ = transcription.ResultFromJSON(cache.TranscriptionJSON)
		}
	}

	// Транскрибируем (или используем кэш)
	var transcriptionResult *transcription.Result
	if cachedTranscription != nil {
		log.Printf("✅ Используем кэшированную транскрипцию для вертикального видео")
		transcriptionResult = cachedTranscription
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "📝 Субтитры из кэша...")
	} else {
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "📝 Создаю субтитры...")
		var err error
		transcriptionResult, err = h.Transcriber.TranscribeAudio(ctx, audioPath)
		if err != nil {
			log.Printf("Failed to transcribe: %v", err)
			transcriptionResult = nil
		}
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

	// Получаем кастомный фон пользователя
	var backgroundPath string
	if h.DB != nil && msg.From != nil {
		_, vertBg, err := h.DB.GetUserBackgrounds(msg.From.ID)
		if err == nil && vertBg != "" {
			backgroundPath = vertBg
		}
	}

	lastUpdatePercent := 0
	videoPath, err := video.GenerateVerticalVideo(
		audioPath,
		avatarPath,
		backgroundPath,
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

	// Сохраняем в кэш с фоном
	if h.DB != nil && voiceFileUniqueID != "" && videoResult != nil {
		videoFileID := ""
		if videoResult.Video != nil {
			videoFileID = videoResult.Video.FileID
		}
		if videoFileID != "" {
			err := h.DB.SaveVerticalCacheWithBackground(voiceFileUniqueID, videoFileID, backgroundPath)
			if err != nil {
				log.Printf("Ошибка сохранения кэша вертикального видео: %v", err)
			} else {
				log.Printf("✅ Сохранён кэш вертикального видео для voice_file_unique_id=%s", voiceFileUniqueID)
			}
		} else {
			log.Printf("⚠️ Не удалось получить file_id вертикального видео")
		}
	}

	// Логируем в БД
	if h.DB != nil {
		h.DB.LogVerticalVideo()
	}

	// Удаляем статусное сообщение
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: statusMsg.ID,
	})
}
