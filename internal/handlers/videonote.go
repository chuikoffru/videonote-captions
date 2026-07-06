package handlers

import (
	"context"
	"log"
	"os"

	"telegram-voice-video-bot/internal/audio"
	"telegram-voice-video-bot/internal/video"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// VideoNoteHandler обрабатывает Video Note сообщения (кружочки)
func (h *Handler) VideoNoteHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.VideoNote == nil {
		return
	}

	msg := update.Message
	userID := msg.From.ID
	chatID := msg.Chat.ID
	videoNote := msg.VideoNote

	// 1. Проверка прав и баланса
	isAdmin := userID == h.Config.AdminID
	cost := int64(h.Config.VideoNoteCredits)
	useFreeNote := false

	if !isAdmin && h.DB != nil {
		// Сначала проверяем бесплатные кружочки
		freeRemaining, err := h.DB.GetFreeVideoNotesRemaining(userID, h.Config.FreeVideoNotesPerMonth)
		if err != nil {
			log.Printf("Failed to get free video notes: %v", err)
			// Продолжаем с проверкой кредитов
			freeRemaining = 0
		}

		if freeRemaining > 0 {
			useFreeNote = true
		} else {
			// Бесплатные кружочки закончились, проверяем баланс кредитов
			balance, err := h.DB.GetUserCredits(userID)
			if err != nil {
				log.Printf("Failed to get user credits: %v", err)
				b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   "❌ Ошибка базы данных.",
				})
				return
			}

			if balance < cost {
				// Показываем сообщение об исчерпании бесплатных кружочков и недостатке кредитов
				h.sendFreeNotesExhaustedMessage(ctx, b, chatID, balance)
				return
			}
			// Если кредитов хватает - продолжаем обработку, они будут списаны после
		}
	}

	// Отправляем статус
	statusMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "⏳ Обрабатываю видео-сообщение...",
		ReplyParameters: &models.ReplyParameters{
			MessageID: msg.ID,
		},
	})
	if err != nil {
		log.Printf("Failed to send status: %v", err)
		return
	}

	// 2. Скачивание Video Note
	h.updateStatus(ctx, b, chatID, statusMsg.ID, "⬇️ Скачиваю видео...")
	videoPath, err := audio.DownloadVideoNote(ctx, b, videoNote.FileID, h.Config.BotToken)
	if err != nil {
		log.Printf("Failed to download video note: %v", err)
		h.updateStatus(ctx, b, chatID, statusMsg.ID, "❌ Ошибка скачивания видео.")
		return
	}
	defer os.Remove(videoPath)

	// 3. Извлечение аудио для транскрипции
	h.updateStatus(ctx, b, chatID, statusMsg.ID, "🔊 Извлекаю звук...")
	audioPath, err := audio.ExtractAudioFromVideo(videoPath)
	if err != nil {
		log.Printf("Failed to extract audio: %v", err)
		h.updateStatus(ctx, b, chatID, statusMsg.ID, "❌ Ошибка обработки аудио.")
		return
	}
	defer os.Remove(audioPath)

	// 4. Транскрипция
	h.updateStatus(ctx, b, chatID, statusMsg.ID, "📝 Распознаю речь...")
	transcriptionResult, err := h.Transcriber.TranscribeAudio(ctx, audioPath)
	if err != nil {
		log.Printf("Failed to transcribe: %v", err)
		h.updateStatus(ctx, b, chatID, statusMsg.ID, "❌ Ошибка распознавания речи.")
		return
	}

	if len(transcriptionResult.Words) == 0 {
		h.updateStatus(ctx, b, chatID, statusMsg.ID, "⚠️ Речь не распознана или видео без слов.")
		return
	}

	// 5. Наложение субтитров
	h.updateStatus(ctx, b, chatID, statusMsg.ID, "🎬 Накладываю субтитры...")
	outputVideoPath, err := video.AddSubtitlesToVideoNote(videoPath, transcriptionResult)
	if err != nil {
		log.Printf("Failed to add subtitles: %v", err)
		h.updateStatus(ctx, b, chatID, statusMsg.ID, "❌ Ошибка создания видео с субтитрами.")
		return
	}
	defer os.Remove(outputVideoPath)

	// 6. Списание средств (перед отправкой, но после успешной генерации)
	if !isAdmin && h.DB != nil {
		if useFreeNote {
			// Используем бесплатный кружочек
			if err := h.DB.UseFreeVideoNote(userID); err != nil {
				log.Printf("Failed to use free video note: %v", err)
			}
		} else {
			// Списываем кредиты
			err := h.DB.SpendCredits(userID, cost, "Video Note Karaoke")
			if err != nil {
				log.Printf("Failed to spend credits: %v", err)
			}
		}
	}

	// Логируем создание video note
	if h.DB != nil {
		if err := h.DB.LogVideoNote(); err != nil {
			log.Printf("Failed to log video note: %v", err)
		}
	}

	// 7. Отправка результата
	h.updateStatus(ctx, b, chatID, statusMsg.ID, "📤 Отправляю результат...")
	
	videoFile, err := os.Open(outputVideoPath)
	if err != nil {
		log.Printf("Failed to open output video: %v", err)
		return
	}
	defer videoFile.Close()

	sentNote, err := b.SendVideoNote(ctx, &bot.SendVideoNoteParams{
		ChatID:    chatID,
		VideoNote: &models.InputFileUpload{Filename: "video_note.mp4", Data: videoFile},
		// Duration: videoNote.Duration, // Можно добавить оригинальную длительность
		// Length: videoNote.Length,
	})

	if err != nil {
		log.Printf("Failed to send video note: %v", err)
		h.updateStatus(ctx, b, chatID, statusMsg.ID, "❌ Ошибка отправки результата.")
		return
	}

	// Удаляем статусное сообщение при успехе
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: statusMsg.ID,
	})

	// Всегда отправляем транскрипт отдельным сообщением под кружком
	replyTo := &models.ReplyParameters{MessageID: msg.ID}
	if sentNote != nil {
		replyTo = &models.ReplyParameters{MessageID: sentNote.ID}
	}
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		Text:            "📝 Транскрипт:\n\n" + transcriptionResult.Text,
		ReplyParameters: replyTo,
	}); err != nil {
		log.Printf("Failed to send transcript: %v", err)
	}
}
