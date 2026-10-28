package handlers

import (
	"context"
	"log"
	"os"

	"videonote-captions/internal/audio"
	"videonote-captions/internal/video"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// VideoNoteHandler обрабатывает Video Note сообщения (кружочки)
func (h *Handler) VideoNoteHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.VideoNote == nil || update.Message.From == nil {
		return
	}

	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "Бот занят обработкой. Попробуйте через минуту."})
		return
	}
	msg := update.Message
	userID := msg.From.ID
	chatID := msg.Chat.ID
	videoNote := msg.VideoNote

	if h.DB == nil {
		return
	}
	if _, err := h.DB.GetOrCreateUser(userID, msg.From.Username, msg.From.FirstName, msg.From.LastName); err != nil {
		return
	}
	isAdmin := userID == h.Config.AdminID
	useFreeNote, err := h.DB.ReserveNote(userID, h.Config.FreeVideoNotesPerMonth, h.Config.VideoNoteCredits, isAdmin)
	if err != nil {
		balance, _ := h.DB.GetUserCredits(userID)
		h.sendFreeNotesExhaustedMessage(ctx, b, chatID, balance)
		return
	}
	delivered := false
	defer func() {
		if !delivered {
			if err := h.DB.RefundNote(userID, useFreeNote, h.Config.VideoNoteCredits, isAdmin); err != nil {
				log.Printf("Refund failed for user %d: %v", userID, err)
			}
		}
	}()
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

	delivered = true
	if sentNote != nil && sentNote.VideoNote != nil {
		if err := h.DB.LogNote(userID, videoNote.Duration, sentNote.VideoNote.FileID, useFreeNote); err != nil {
			log.Printf("Log note: %v", err)
		}
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
