package audio

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-telegram/bot"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"videonote-captions/internal/httpclient"
)

var ErrFileTooLarge = errors.New("video exceeds download size limit")

func DownloadTelegramVideo(ctx context.Context, b *bot.Bot, fileID, botToken, workDir string, maxBytes int64) (string, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return "", errors.New("could not retrieve Telegram file")
	}
	if file.FileSize > maxBytes {
		return "", ErrFileTooLarge
	}
	if file.FilePath == "" {
		return "", errors.New("Telegram file has no path")
	}
	target := filepath.Join(workDir, "input.mp4")
	rawURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, file.FilePath)
	if err = downloadTo(ctx, httpclient.Client(), rawURL, target, maxBytes); err != nil {
		return "", err
	}
	return target, nil
}

func downloadTo(ctx context.Context, client *http.Client, rawURL, target string, maxBytes int64) (err error) {
	if maxBytes <= 0 {
		return ErrFileTooLarge
	}
	defer func() {
		if err != nil {
			os.Remove(target)
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return errors.New("invalid download request")
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("video download request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("video download status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return ErrFileTooLarge
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(resp.Body, maxBytes+1))
	closeErr := out.Close()
	if n > maxBytes {
		return ErrFileTooLarge
	}
	if copyErr != nil {
		return errors.New("incomplete video download")
	}
	if closeErr != nil {
		return closeErr
	}
	if n == 0 {
		return errors.New("empty video")
	}
	return nil
}
