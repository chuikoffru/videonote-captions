package audio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-telegram/bot"
)

// DownloadAudio скачивает голосовое сообщение из Telegram
func DownloadAudio(ctx context.Context, b *bot.Bot, fileID, botToken string) (string, error) {
	// Получаем информацию о файле
	file, err := b.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get file: %w", err)
	}

	// Создаём temp директорию
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Формируем URL для скачивания
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, file.FilePath)

	// Скачиваем файл
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	// Сохраняем в файл
	audioPath := filepath.Join(tempDir, fmt.Sprintf("audio_%d.ogg", time.Now().UnixNano()))
	out, err := os.Create(audioPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	return audioPath, nil
}
