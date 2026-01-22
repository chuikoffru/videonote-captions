package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"telegram-voice-video-bot/internal/transcription"
)

// AddSubtitlesToVideoNote накладывает субтитры с эффектом караоке на Video Note
func AddSubtitlesToVideoNote(videoPath string, transcriptionResult *transcription.Result) (string, error) {
	tempDir := filepath.Dir(videoPath)
	outputPath := filepath.Join(tempDir, fmt.Sprintf("karaoke_videonote_%d.mp4", time.Now().UnixNano()))

	// Создаём ASS файл субтитров с караоке
	assPath, err := createKaraokeASS(transcriptionResult, tempDir)
	if err != nil {
		return "", fmt.Errorf("failed to create ASS subtitles: %w", err)
	}
	// defer os.Remove(assPath) // Оставляем для дебага пока что

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

// createKaraokeASS создаёт ASS файл с караоке подсветкой
func createKaraokeASS(result *transcription.Result, tempDir string) (string, error) {
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
Style: Karaoke,Arial,52,&H0000FFFF,&H00FFFFFF,&H00000000,&H80000000,1,0,0,0,100,100,0,0,1,3,0,2,40,40,50,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`
	if _, err := file.WriteString(header); err != nil {
		return "", err
	}

	if result == nil || len(result.Words) == 0 {
		return assPath, nil // Пустой файл если нет слов
	}

	// Группируем слова в фразы (по 3-5 слов или по паузам)
	type karaokeWord struct {
		Text     string
		Duration int // в сантисекундах (для \k тега)
	}
	type karaokePhrase struct {
		Start float64
		End   float64
		Words []karaokeWord
		Raw   string // Для проверки
	}

	var phrases []karaokePhrase
	var currentPhrase karaokePhrase
	
	maxWordsPerPhrase := 3
	maxPhraseDuration := 2.5

	// Инициализируем первую фразу
	if len(result.Words) > 0 {
		currentPhrase.Start = result.Words[0].Start
	}

	for i, word := range result.Words {
		// Длительность слова в сантисекундах (1 cs = 10 ms) для тега \k
		duration := word.End - word.Start
		// Коррекция: иногда Whisper выдает наложение или разрывы.
		// Эвристика: если есть следующее слово, длительность может быть до начала следующего
		// Но \k работает внутри одной строки.
		
		kDuration := int(duration * 100)
		if kDuration < 1 {
			kDuration = 1
		}

		currentPhrase.Words = append(currentPhrase.Words, karaokeWord{
			Text:     word.Word,
			Duration: kDuration,
		})

		// Условие разрыва фразы
		shouldBreak := len(currentPhrase.Words) >= maxWordsPerPhrase ||
			(word.End-currentPhrase.Start) >= maxPhraseDuration ||
			i == len(result.Words)-1

		if shouldBreak {
			currentPhrase.End = word.End
			phrases = append(phrases, currentPhrase)
			
			// Начинаем новую фразу
			currentPhrase = karaokePhrase{
				Words: []karaokeWord{},
			}
			if i < len(result.Words)-1 {
				currentPhrase.Start = result.Words[i+1].Start
			}
		}
	}

	// Записываем события в файл
	for _, p := range phrases {
		startTime := formatASSTime(p.Start)
		endTime := formatASSTime(p.End)
		
		var lineDialog strings.Builder
		// {\k10}Word {\k20}Another
		
		// Вторичный цвет (SecondaryColour) используется для "заливки" в караоке (\k).
		// В ASS: PrimaryColour - цвет уже спетого (или наоборот, зависит от плеера/рендерера, но обычно Primary - основной, Secondary - для караоке ДО спетия).
		// Стандартный \k: цвет меняется слева направо. 
		// Обычно Primary - это "будущий" цвет, а Secondary - "прошедший". Или наоборот.
		// Проверим: \k закрашивает SecondaryColor поверх PrimaryColor. 
		// Точнее: Текст отображается PrimaryColour. \k эффект закрашивает его SecondaryColour.
		// Нам нужно: Неактивное слово (белое), активное/спетое (синее/желтое).
		// В Style: Primary=&H00FFFFFF (Белый), Secondary=&H0000FFFF (Желтый).
		// Эффект \kf или \K - плавная заливка. \k - блочная.
		
		// Формируем строку с тегами
		lineDialog.WriteString("{\\k0}") // Начальный сброс
		for _, w := range p.Words {
			// добавляем пробел перед словом если это не первое слово, но в караоке это сложно.
			// Обычно слова уже содержат пробелы или мы их добавляем.
			// Whisper обычно дает слова без пробелов.
			
			// Используем \kf для плавной заливки (хайлайта)
			lineDialog.WriteString(fmt.Sprintf("{\\kf%d}%s ", w.Duration, w.Text))
		}

		// Dialogue: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
		line := fmt.Sprintf("Dialogue: 0,%s,%s,Karaoke,,0,0,0,,%s\n", startTime, endTime, lineDialog.String())
		if _, err := file.WriteString(line); err != nil {
			return "", err
		}
	}

	return assPath, nil
}

// formatASSTime форматирует время в ASS формат 0:00:00.00
func formatASSTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	cs := int((seconds - float64(int(seconds))) * 100) // centiseconds

	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, secs, cs)
}
