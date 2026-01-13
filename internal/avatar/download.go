package avatar

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-telegram/bot"
)

// Download скачивает аватар пользователя или чата/канала
func Download(ctx context.Context, b *bot.Bot, targetID int64, targetType, botToken string) (string, error) {
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Если ID не передан, создаём дефолтную аватарку
	if targetID == 0 {
		return createDefaultAvatar(tempDir)
	}

	var fileID string

	switch targetType {
	case "chat", "channel":
		chat, err := b.GetChat(ctx, &bot.GetChatParams{
			ChatID: targetID,
		})
		if err != nil {
			return createDefaultAvatar(tempDir)
		}
		if chat.Photo != nil {
			fileID = chat.Photo.BigFileID
		}
	default: // "user"
		photos, err := b.GetUserProfilePhotos(ctx, &bot.GetUserProfilePhotosParams{
			UserID: targetID,
			Limit:  1,
		})
		if err != nil {
			return createDefaultAvatar(tempDir)
		}
		if photos.TotalCount > 0 && len(photos.Photos) > 0 && len(photos.Photos[0]) > 0 {
			// Берём самое большое фото
			photoSizes := photos.Photos[0]
			fileID = photoSizes[len(photoSizes)-1].FileID
		}
	}

	if fileID == "" {
		return createDefaultAvatar(tempDir)
	}

	// Получаем файл
	file, err := b.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		return createDefaultAvatar(tempDir)
	}

	// Скачиваем файл
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, file.FilePath)
	resp, err := http.Get(fileURL)
	if err != nil {
		return createDefaultAvatar(tempDir)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return createDefaultAvatar(tempDir)
	}

	avatarPath := filepath.Join(tempDir, fmt.Sprintf("avatar_%d.jpg", time.Now().UnixNano()))
	out, err := os.Create(avatarPath)
	if err != nil {
		return createDefaultAvatar(tempDir)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return createDefaultAvatar(tempDir)
	}

	return avatarPath, nil
}

// createDefaultAvatar создаёт дефолтную аватарку с градиентом
func createDefaultAvatar(tempDir string) (string, error) {
	const size = 200
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Создаём градиентный фон (cyan -> magenta)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			ratio := float64(x+y) / float64(2*size)
			r := uint8(0 + 255*ratio)
			g := uint8(255 - 255*ratio)
			b := uint8(255)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Рисуем белый силуэт пользователя
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	// Голова (круг)
	drawCircle(img, size/2, 85, 40, white)

	// Тело (полукруг снизу)
	drawSemiCircle(img, size/2, size, 80, white)

	// Сохраняем
	avatarPath := filepath.Join(tempDir, fmt.Sprintf("avatar_default_%d.png", time.Now().UnixNano()))
	f, err := os.Create(avatarPath)
	if err != nil {
		return "", fmt.Errorf("failed to create default avatar file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return "", fmt.Errorf("failed to encode default avatar: %w", err)
	}

	return avatarPath, nil
}

// drawCircle рисует заполненный круг
func drawCircle(img *image.RGBA, cx, cy, r int, c color.Color) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				img.Set(x, y, c)
			}
		}
	}
}

// drawSemiCircle рисует верхнюю полусферу (для тела)
func drawSemiCircle(img *image.RGBA, cx, cy, r int, c color.Color) {
	for y := cy - r; y <= cy; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				img.Set(x, y, c)
			}
		}
	}
}

// Ensure draw package is imported for interface
var _ draw.Image = (*image.RGBA)(nil)
