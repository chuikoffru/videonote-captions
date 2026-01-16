package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"telegram-voice-video-bot/internal/transcription"
)

// ProgressCallback функция обратного вызова для прогресса
type ProgressCallback func(percent int)

// VideoConfig содержит параметры для генерации видео
type VideoConfig struct {
	AudioPath      string
	AvatarPath     string
	BackgroundPath string // Путь к кастомному фону (опционально)
	Transcription  *transcription.Result
	Username       string
	Width          int
	Height         int
	FPS            int
	Bitrate        string
	OnProgress     ProgressCallback
	OutputPrefix   string
}

// FontConfig содержит пути и имена шрифтов
type FontConfig struct {
	Path string
	Name string
}

// getFontConfig возвращает конфигурацию шрифта для текущей ОС
func getFontConfig() FontConfig {
	if runtime.GOOS == "darwin" {
		return FontConfig{
			Path: "/System/Library/Fonts/Helvetica.ttc",
			Name: "Helvetica",
		}
	}
	return FontConfig{
		Path: "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
		Name: "Noto Sans",
	}
}

// formatUsername форматирует юзернейм с @
func formatUsername(username string) string {
	if !strings.HasPrefix(username, "@") {
		return "@" + username
	}
	return username
}

// escapeSubtitlePath экранирует путь к субтитрам для FFmpeg
func escapeSubtitlePath(path string) string {
	escaped := strings.ReplaceAll(path, ":", "\\:")
	return strings.ReplaceAll(escaped, "\\", "/")
}

// generateVideoWithFilter создаёт видео с указанным complex filter
func generateVideoWithFilter(cfg VideoConfig, complexFilter string) (string, error) {
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	outputPath := filepath.Join(tempDir, fmt.Sprintf("%s_%d.mp4", cfg.OutputPrefix, time.Now().UnixNano()))

	// Базовые аргументы
	args := []string{
		"-i", cfg.AudioPath,
		"-i", cfg.AvatarPath,
	}

	// Добавляем фон как третий input если он указан
	if cfg.BackgroundPath != "" {
		args = append(args, "-i", cfg.BackgroundPath)
	}

	args = append(args,
		"-filter_complex", complexFilter,
		"-map", "[final]",
		"-map", "0:a",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-b:v", cfg.Bitrate,
		"-r", strconv.Itoa(cfg.FPS),
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		"-y",
		outputPath,
	)

	return runFFmpegWithProgress(args, cfg.AudioPath, cfg.OnProgress, outputPath)
}

// runFFmpegWithProgress запускает FFmpeg с отслеживанием прогресса
func runFFmpegWithProgress(args []string, audioPath string, onProgress ProgressCallback, outputPath string) (string, error) {
	cmd := exec.Command("ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	progressRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)
	lastPercent := 0
	buf := make([]byte, 256)
	var accumulated string

	audioDuration := getAudioDuration(audioPath)
	if audioDuration <= 0 {
		audioDuration = 60 // fallback
	}

	go func() {
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				accumulated += string(buf[:n])

				if matches := progressRegex.FindStringSubmatch(accumulated); matches != nil {
					hours, _ := strconv.Atoi(matches[1])
					minutes, _ := strconv.Atoi(matches[2])
					seconds, _ := strconv.Atoi(matches[3])
					totalSeconds := hours*3600 + minutes*60 + seconds

					percent := totalSeconds * 100 / int(audioDuration)
					if percent > 99 {
						percent = 99
					}

					if percent != lastPercent && onProgress != nil {
						onProgress(percent)
						lastPercent = percent
					}

					if len(accumulated) > 200 {
						accumulated = accumulated[len(accumulated)-100:]
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w", err)
	}

	if onProgress != nil {
		onProgress(100)
	}

	return outputPath, nil
}

// getAudioDuration получает длительность аудио в секундах через ffprobe
func getAudioDuration(audioPath string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", audioPath)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0
	}
	return duration
}

// createSubtitleFile создаёт SRT файл из транскрипции
func createSubtitleFile(result *transcription.Result, outputPath string) error {
	if result == nil {
		return os.WriteFile(outputPath, []byte("1\n00:00:00,000 --> 00:00:05,000\n \n\n"), 0644)
	}

	words := result.Words
	if len(words) == 0 {
		text := result.Text
		if text == "" {
			text = " "
		}
		content := fmt.Sprintf("1\n00:00:00,000 --> 00:00:05,000\n%s\n\n", text)
		return os.WriteFile(outputPath, []byte(content), 0644)
	}

	// Группируем слова в фразы
	type phrase struct {
		text  string
		start float64
		end   float64
	}

	var phrases []phrase
	var currentWords []string
	phraseStart := words[0].Start
	maxWordsPerPhrase := 6
	maxPhraseDuration := 3.0

	for i, word := range words {
		currentWords = append(currentWords, word.Word)

		shouldBreak := len(currentWords) >= maxWordsPerPhrase ||
			(word.End-phraseStart) >= maxPhraseDuration ||
			i == len(words)-1

		if shouldBreak {
			phrases = append(phrases, phrase{
				text:  strings.TrimSpace(strings.Join(currentWords, " ")),
				start: phraseStart,
				end:   word.End,
			})
			currentWords = nil
			if i < len(words)-1 {
				phraseStart = words[i+1].Start
			}
		}
	}

	// Генерируем SRT
	var content strings.Builder
	for i, p := range phrases {
		startTime := formatSRTTime(p.start)
		endTime := formatSRTTime(p.end)
		content.WriteString(fmt.Sprintf("%d\n%s --> %s\n%s\n\n", i+1, startTime, endTime, p.text))
	}

	return os.WriteFile(outputPath, []byte(content.String()), 0644)
}

// formatSRTTime форматирует время в SRT формат
func formatSRTTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, ms)
}

// prepareSubtitles создаёт файл субтитров и возвращает путь
func prepareSubtitles(transcription *transcription.Result, tempDir string) (string, string, error) {
	subtitlePath := filepath.Join(tempDir, fmt.Sprintf("subtitles_%d.srt", time.Now().UnixNano()))
	if err := createSubtitleFile(transcription, subtitlePath); err != nil {
		return "", "", fmt.Errorf("failed to create subtitles: %w", err)
	}
	return subtitlePath, escapeSubtitlePath(subtitlePath), nil
}
