package video

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"videonote-captions/internal/transcription"
)

// AddSubtitlesToVideoNote накладывает субтитры по одному слову на Video Note
func AddSubtitlesToVideoNote(videoPath string, transcriptionResult *transcription.Result) (string, error) {
	tempDir := filepath.Dir(videoPath)
	outputPath := filepath.Join(tempDir, fmt.Sprintf("captioned_videonote_%d.mp4", time.Now().UnixNano()))

	// Создаём ASS файл субтитров
	assPath, err := createWordASS(transcriptionResult, tempDir)
	if err != nil {
		return "", fmt.Errorf("failed to create ASS subtitles: %w", err)
	}
	defer os.Remove(assPath)

	// Экранируем путь для фильтра
	escapedASSPath := escapeSubtitlePath(assPath)

	// Накладываем субтитры
	// Используем subtitles фильтр с указанием ASS файла
	// Video Note в Telegram имеет размер до 384x384 (или 640x640), поэтому шрифт подбираем среднего размера
	// Важно: Сохраняем кодек H.264 и формат пикселей yuv420p для совместимости
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vf", fmt.Sprintf("subtitles='%s'", escapedASSPath),
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		"-c:a", "copy", // Аудио оставляем без изменений
		"-y",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %v, output: %s", err, string(output))
	}

	return outputPath, nil
}

// createWordASS создаёт ASS файл с отдельным событием для каждого слова
func createWordASS(result *transcription.Result, tempDir string) (string, error) {
	assPath := filepath.Join(tempDir, fmt.Sprintf("subs_%d.ass", time.Now().UnixNano()))
	file, err := os.Create(assPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Заголовок ASS файла
	// Стиль: белый шрифт с черной обводкой, alignment 2 (внизу по центру), без фона
	header := `[Script Info]
ScriptType: v4.00+
PlayResX: 384
PlayResY: 384
WrapStyle: 0
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Word,Noto Sans,52,&H00FFFFFF,&H00FFFFFF,&H00000000,&H80000000,1,0,0,0,100,100,0,0,1,3,0,2,40,40,50,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`
	if _, err := file.WriteString(header); err != nil {
		return "", err
	}

	if result == nil || len(result.Words) == 0 {
		return assPath, nil // Пустой файл если нет слов
	}

	for i, word := range result.Words {
		start, end := word.Start, word.End
		text := strings.TrimSpace(word.Word)
		if text == "" || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || start < 0 || end <= start {
			continue
		}
		for j := i + 1; j < len(result.Words); j++ {
			next := result.Words[j]
			if strings.TrimSpace(next.Word) != "" && next.End > next.Start && next.Start >= start {
				end = math.Min(end, next.Start)
				break
			}
		}
		if end <= start {
			continue
		}
		// Neutralize ASS control sequences supplied by a transcription.
		text = strings.NewReplacer("\\", "／", "{", "(", "}", ")", "\n", " ", "\r", " ").Replace(text)
		if _, err := fmt.Fprintf(file, "Dialogue: 0,%s,%s,Word,,0,0,0,,%s\n", formatASSTime(start), formatASSTime(end), text); err != nil {
			return "", err
		}
	}

	return assPath, nil
}

// formatASSTime форматирует время в ASS формат 0:00:00.00
func formatASSTime(seconds float64) string {
	seconds = math.Round(seconds*100) / 100
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	cs := int(math.Round(seconds*100)) % 100 // centiseconds

	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, secs, cs)
}

func escapeSubtitlePath(path string) string {
	return strings.NewReplacer("\\", "/", ":", "\\:", "'", "\\'", "[", "\\[", "]", "\\]", ",", "\\,").Replace(path)
}
