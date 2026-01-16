package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"telegram-voice-video-bot/internal/audio"
	"telegram-voice-video-bot/internal/avatar"
	"telegram-voice-video-bot/internal/transcription"
	"telegram-voice-video-bot/internal/video"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// VoiceHandler обрабатывает голосовые сообщения
func (h *Handler) VoiceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Voice == nil {
		return
	}

	h.handleVoiceMessage(ctx, b, update.Message, update.Message.Voice, false, nil, update.Message.Voice.FileUniqueID)
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
	var cachedTranscription *transcription.Result
	if h.DB != nil && voiceFileUniqueID != "" {
		cache, err := h.DB.GetCachedVideo(voiceFileUniqueID)
		if err != nil {
			log.Printf("Ошибка проверки кэша: %v", err)
		} else if cache != nil {
			// Получаем текущий фон пользователя
			var currentHorizBg string
			if msg.From != nil {
				currentHorizBg, _, _ = h.DB.GetUserBackgrounds(msg.From.ID)
			}

			// Проверяем, изменился ли фон
			backgroundChanged := cache.HorizontalBackground != currentHorizBg

			// Если есть кэшированная транскрипция, сохраняем для использования
			if cache.TranscriptionJSON != "" {
				cachedTranscription, _ = transcription.ResultFromJSON(cache.TranscriptionJSON)
			}

			// Если видео есть в кэше И фон не изменился
			if cache.VideoFileID != "" && !backgroundChanged {
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

				// Определяем, есть ли вертикальное видео в кэше
				var buttonText, buttonCallback string
				if cache.VerticalVideoFileID != "" {
					buttonText = "📱 Получить вертикальное видео (бесплатно)"
					buttonCallback = "vertcache:" + vertKey
				} else {
					buttonText = fmt.Sprintf("📱 Сделать вертикальное видео (%d⭐)", h.Config.VerticalCost)
					buttonCallback = "vert:" + vertKey
				}

				// Отправляем видео из кэша по file_id
				_, err := b.SendVideo(ctx, &bot.SendVideoParams{
					ChatID: msg.Chat.ID,
					Video:  &models.InputFileString{Data: cache.VideoFileID},
					ReplyMarkup: &models.InlineKeyboardMarkup{
						InlineKeyboard: [][]models.InlineKeyboardButton{
							{
								{
									Text:         buttonText,
									CallbackData: buttonCallback,
								},
							},
						},
					},
				})
				if err != nil {
					log.Printf("Ошибка отправки видео из кэша: %v", err)
				} else {
					return // Успешно отправили из кэша
				}
			} else if backgroundChanged && cache.VideoFileID != "" {
				log.Printf("Фон изменился, инвалидируем кэш видео")
				h.DB.InvalidateVideoCache(voiceFileUniqueID, true, true)
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

	// Транскрибируем (или используем кэш)
	var transcriptionResult *transcription.Result
	if cachedTranscription != nil {
		log.Printf("✅ Используем кэшированную транскрипцию")
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
		h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "❌ Произошла ошибка при обработке. Попробуйте ещё раз.")
		return
	}
	defer os.Remove(avatarPath)

	// Генерируем видео
	h.updateStatus(ctx, b, msg.Chat.ID, statusMsg.ID, "🎬 Создаю видео: 0%")

	lastUpdatePercent := 0
	log.Printf("Video Target: %s", author.Username)

	// Получаем кастомный фон пользователя
	var backgroundPath string
	if h.DB != nil && msg.From != nil {
		horizBg, _, err := h.DB.GetUserBackgrounds(msg.From.ID)
		if err == nil && horizBg != "" {
			backgroundPath = horizBg
		}
	}

	videoPath, err := video.GenerateVideo(
		audioPath,
		avatarPath,
		backgroundPath,
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
		Type:         "vertical",
		FileID:       voice.FileID,
		FileUniqueID: voiceFileUniqueID,
		Author:       *author,
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

		// Сохраняем в кэш с транскрипцией и фоном
		if voiceFileUniqueID != "" && videoFileID != "" {
			transcriptionJSON := ""
			if transcriptionResult != nil {
				transcriptionJSON, _ = transcriptionResult.ToJSON()
			}
			err := h.DB.SaveCacheWithTranscription(voiceFileUniqueID, videoFileID, transcriptionJSON, backgroundPath)
			if err != nil {
				log.Printf("Ошибка сохранения кэша видео: %v", err)
			} else {
				log.Printf("✅ Сохранён кэш видео с транскрипцией для voice_file_unique_id=%s", voiceFileUniqueID)
			}
		}
	}

	// Удаляем статусное сообщение
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: statusMsg.ID,
	})
}
