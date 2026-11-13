package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func ExtractAudioFromVideo(videoPath string) (string, error) {
	return ExtractAudioFromVideoContext(context.Background(), videoPath)
}

func ExtractAudioFromVideoContext(ctx context.Context, videoPath string) (string, error) {
	path := filepath.Join(filepath.Dir(videoPath), "speech.mp3")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", videoPath, "-vn", "-map", "0:a:0", "-ac", "1", "-ar", "16000", "-c:a", "libmp3lame", "-b:a", "64k", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("audio extraction failed: %w (%s)", err, output)
	}
	return path, nil
}
