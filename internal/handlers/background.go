package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"telegram-voice-video-bot/internal/httpclient"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// PhotoHandler обрабатывает загруженные фотографии для установки кастомного фона
func (h *Handler) PhotoHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Photo == nil || len(update.Message.Photo) == 0 {
		return
	}

	// Регистрируем пользователя если ещё не зарегистрирован
	if h.DB != nil && update.Message.From != nil {
		h.DB.GetOrCreateUser(
			update.Message.From.ID,
			update.Message.From.Username,
			update.Message.From.FirstName,
			update.Message.From.LastName,
		)
	}

	// Берём самую большую версию фото
	photos := update.Message.Photo
	photo := photos[len(photos)-1]

	// Определяем ориентацию
	isHorizontal := photo.Width > photo.Height
	orientationType := "вертикальный"
	suffix := "v"
	if isHorizontal {
		orientationType = "горизонтальный"
		suffix = "h"
	}

	// Скачиваем фото
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: photo.FileID})
	if err != nil {
		log.Printf("Failed to get file: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка получения файла.",
		})
		return
	}

	// Создаём директорию для фонов
	bgDir := "data/backgrounds"
	if err := os.MkdirAll(bgDir, 0755); err != nil {
		log.Printf("Failed to create backgrounds dir: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка сохранения файла.",
		})
		return
	}

	// Формируем путь для сохранения
	filename := fmt.Sprintf("%d_%s.jpg", update.Message.From.ID, suffix)
	savePath := filepath.Join(bgDir, filename)

	// Скачиваем и сохраняем файл
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.Config.BotToken, file.FilePath)
	if err := downloadFile(fileURL, savePath); err != nil {
		log.Printf("Failed to download file: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка скачивания файла.",
		})
		return
	}

	// Обновляем запись в БД
	if h.DB != nil {
		if err := h.DB.UpdateUserBackground(update.Message.From.ID, isHorizontal, savePath); err != nil {
			log.Printf("Failed to update user background: %v", err)
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "❌ Ошибка сохранения настроек.",
			})
			return
		}
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("✅ Установлен новый %s фон!\n\nТеперь все твои видео будут с этим фоном.", orientationType),
	})
}

// downloadFile скачивает файл по URL и сохраняет локально
func downloadFile(url, filepath string) error {
	resp, err := httpclient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
