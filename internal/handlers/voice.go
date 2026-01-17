package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"telegram-voice-video-bot/internal/audio"
	"telegram-voice-video-bot/internal/avatar"
	"telegram-voice-video-bot/internal/transcription"
	"telegram-voice-video-bot/internal/video"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// VoiceHandler обрабатывает голосовые сообщения — показывает выбор типа видео
func (h *Handler) VoiceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Voice == nil {
		return
	}

	msg := update.Message
	voice := msg.Voice

	// Регистрируем пользователя если ещё не зарегистрирован
	if h.DB != nil && msg.From != nil {
		h.DB.GetOrCreateUser(
			msg.From.ID,
			msg.From.Username,
			msg.From.FirstName,
			msg.From.LastName,
		)
	}

	// Определяем автора голосового сообщения
	author := h.resolveVoiceAuthor(msg)

	// Проверяем, является ли пользователь админом (админ без ограничений)
	isAdmin := msg.From != nil && msg.From.ID == h.Config.AdminID

	// Сохраняем данные для последующей обработки callback
	verticalRequestCounter++
	videoKey := fmt.Sprintf("v%d", verticalRequestCounter)
	VerticalRequests.Store(videoKey, OrderData{
		Type:         "voice",
		FileID:       voice.FileID,
		FileUniqueID: voice.FileUniqueID,
		Author:       *author,
	})

	// Рассчитываем стоимость
	duration := voice.Duration
	freeSeconds := h.Config.FreeSeconds
	paidSeconds := duration - freeSeconds
	if paidSeconds < 0 {
		paidSeconds = 0
	}
	paidMinutes := (paidSeconds + 59) / 60 // Округление вверх
	creditsCost := paidMinutes * h.Config.CreditsPerMinute

	// Формируем текст сообщения
	var messageText string
	var horizButtonText, vertButtonText string

	durationMin := duration / 60
	durationSec := duration % 60

	if isAdmin {
		messageText = fmt.Sprintf(
			"🎤 <b>Голосовое сообщение</b>\n\n"+
				"⏱ Длительность: %d:%02d\n"+
				"👑 Вы администратор — генерация бесплатна\n\n"+
				"Выберите формат видео:",
			durationMin, durationSec,
		)
		horizButtonText = "📺 Горизонтальное видео"
		vertButtonText = "📱 Вертикальное видео"
	} else if paidMinutes == 0 {
		messageText = fmt.Sprintf(
			"🎤 <b>Голосовое сообщение</b>\n\n"+
				"⏱ Длительность: %d:%02d\n"+
				"✅ Первые %d секунд бесплатно\n\n"+
				"Выберите формат видео:",
			durationMin, durationSec, freeSeconds,
		)
		horizButtonText = "📺 Горизонтальное видео"
		vertButtonText = "📱 Вертикальное видео"
	} else {
		messageText = fmt.Sprintf(
			"🎤 <b>Голосовое сообщение</b>\n\n"+
				"⏱ Длительность: %d:%02d\n"+
				"✅ Первые %d секунд — бесплатно\n"+
				"💰 Дополнительные %d мин. — %d %s\n\n"+
				"Выберите формат видео:",
			durationMin, durationSec,
			freeSeconds,
			paidMinutes, creditsCost, formatCreditsWord(int64(creditsCost)),
		)
		horizButtonText = fmt.Sprintf("📺 Горизонтальное (%d кред.)", creditsCost)
		vertButtonText = fmt.Sprintf("📱 Вертикальное (%d кред.)", creditsCost)
	}

	// Отправляем сообщение с кнопками
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      messageText,
		ParseMode: models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{
			MessageID: msg.ID,
		},
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text:         horizButtonText,
						CallbackData: "horiz:" + videoKey,
					},
				},
				{
					{
						Text:         vertButtonText,
						CallbackData: "vert:" + videoKey,
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("Failed to send video type selection: %v", err)
	}
}

// handleVoiceMessage обрабатывает голосовое сообщение
// cachedTranscription может быть передан вызывающим кодом, чтобы избежать повторной транскрипции
func (h *Handler) handleVoiceMessage(ctx context.Context, b *bot.Bot, msg *models.Message, voice *models.Voice, isPaid bool, authorOverride *Author, voiceFileUniqueID string, cachedTranscription *transcription.Result) {
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

	// Проверяем кэш перед обработкой (только если транскрипция не была передана)
	if cachedTranscription == nil && h.DB != nil && voiceFileUniqueID != "" {
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

			// Если есть кэшированная транскрипция, загружаем для использования
			if cache.TranscriptionJSON != "" {
				cachedTranscription, _ = transcription.ResultFromJSON(cache.TranscriptionJSON)
			}

			// Если видео есть в кэше И фон не изменился
			if cache.VideoFileID != "" && !backgroundChanged {
				log.Printf("✅ Найден кэш для voice_file_unique_id=%s, отправляем видео из кэша", voiceFileUniqueID)

				// Отправляем видео из кэша по file_id (без кнопки)
				_, err := b.SendVideo(ctx, &bot.SendVideoParams{
					ChatID: msg.Chat.ID,
					Video:  &models.InputFileString{Data: cache.VideoFileID},
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
	_ = msg.From != nil && msg.From.ID == h.Config.AdminID // isAdmin - не используется, так как оплата через кредиты проверяется в callback

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

	videoResult, err := b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: msg.Chat.ID,
		Video:  &models.InputFileUpload{Filename: "video.mp4", Data: videoFile},
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
