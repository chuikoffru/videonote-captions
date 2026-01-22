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
	"os/exec"
)

// DownloadVideoNote скачивает Video Note (кружочек) из Telegram
func DownloadVideoNote(ctx context.Context, b *bot.Bot, fileID, botToken string) (string, error) {
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

	// Сохраняем в файл mp4
	videoPath := filepath.Join(tempDir, fmt.Sprintf("videonote_%d.mp4", time.Now().UnixNano()))
	out, err := os.Create(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	return videoPath, nil
}

// ExtractAudioFromVideo извлекает аудио из видеофайла и сохраняет в mp3
func ExtractAudioFromVideo(videoPath string) (string, error) {
	tempDir := filepath.Dir(videoPath)
	audioPath := filepath.Join(tempDir, fmt.Sprintf("extracted_audio_%d.mp3", time.Now().UnixNano()))

	cmd := exec.Command("ffmpeg", "-i", videoPath, "-q:a", "0", "-map", "a", audioPath, "-y")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %v, output: %s", err, string(output))
	}

	return audioPath, nil
}

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
