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

// GenerateVideo создаёт видео с визуализацией аудио, аватаркой и субтитрами
func GenerateVideo(audioPath, avatarPath string, transcriptionResult *transcription.Result, username string, width, height, fps int, bitrate string, onProgress ProgressCallback) (string, error) {
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	outputPath := filepath.Join(tempDir, fmt.Sprintf("video_%d.mp4", time.Now().UnixNano()))

	// Создаём SRT файл субтитров
	subtitlePath := filepath.Join(tempDir, fmt.Sprintf("subtitles_%d.srt", time.Now().UnixNano()))
	if err := createSubtitleFile(transcriptionResult, subtitlePath); err != nil {
		return "", fmt.Errorf("failed to create subtitles: %w", err)
	}

	// Определяем шрифт в зависимости от ОС
	var fontPath, fontName string
	if runtime.GOOS == "darwin" {
		fontPath = "/System/Library/Fonts/Helvetica.ttc"
		fontName = "Helvetica"
	} else {
		fontPath = "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf"
		fontName = "Noto Sans"
	}

	// Размеры элементов
	avatarSize := 200
	avatarX := (width - avatarSize) / 2
	waveformHeight := 120

	// Форматируем юзернейм
	displayUsername := username
	if !strings.HasPrefix(displayUsername, "@") {
		displayUsername = "@" + displayUsername
	}

	// Экранируем путь к субтитрам
	escapedSubtitlePath := strings.ReplaceAll(subtitlePath, ":", "\\:")
	escapedSubtitlePath = strings.ReplaceAll(escapedSubtitlePath, "\\", "/")

	// Формируем complex filter
	complexFilter := fmt.Sprintf(
		// 1. Фон
		`color=c=0x1a1a2e:s=%dx%d:d=10[bg];`+
			`[bg]drawbox=x=0:y=0:w=%d:h=%d:color=0x16213e:t=fill[bg_grad];`+

			// 2. Аватар - круглый с рамкой
			`[1:v]scale=%d:%d,format=rgba,`+
			`geq=r='if(gt(hypot(X-%d,Y-%d),%d),0,p(X,Y))':`+
			`g='if(gt(hypot(X-%d,Y-%d),%d),255,p(X,Y))':`+
			`b='if(gt(hypot(X-%d,Y-%d),%d),255,p(X,Y))':`+
			`a='if(gt(hypot(X-%d,Y-%d),%d),0,255)'[avatar_round];`+

			// Накладываем аватар
			`[bg_grad][avatar_round]overlay=%d:80[with_avatar];`+

			// Добавляем юзернейм
			`[with_avatar]drawtext=fontfile=%s:text='%s':fontcolor=white:fontsize=24:x=(w-text_w)/2:y=290[with_username];`+

			// 3. Визуализация
			`[0:a]volume=8.0[loud_a];`+
			`[loud_a]showfreqs=s=64x%d:mode=bar:ascale=log:fscale=log:colors=0x00ffff[freqs_base];`+
			`[freqs_base]format=yuv420p[freqs_base_yuv];`+
			`[freqs_base_yuv]scale=128:%d:flags=neighbor[freqs_x2];`+
			`[freqs_x2]geq=lum='if(mod(X,2),0,p(X,Y))':cb='if(mod(X,2),128,p(X,Y))':cr='if(mod(X,2),128,p(X,Y))'[freqs_holes];`+
			`[freqs_holes]scale=%d:%d:flags=neighbor[freqs_spaced];`+
			`[freqs_spaced]tmix=frames=3[freqs_smooth];`+
			`[freqs_smooth]split[freqs_top][freqs_mirror_src];`+
			`[freqs_mirror_src]vflip[freqs_bottom];`+
			`[freqs_top][freqs_bottom]vstack[freqs_full];`+
			`[freqs_full]colorkey=0x000000:0.01:0.1[freqs_transparent];`+
			`[with_username][freqs_transparent]overlay=50:350[with_wave];`+

			// 4. Субтитры
			`[with_wave]subtitles=%s:force_style='FontName=%s,FontSize=30,PrimaryColour=&HFFFFFF&,BorderStyle=0,Outline=0,Shadow=0,Alignment=2,MarginV=20'[with_subs];`+

			// 5. Имя бота
			`[with_subs]drawtext=fontfile=%s:text='@AudioVisualizerBot':fontcolor=0x00ffff:fontsize=20:x=(w-text_w)/2:y=h-30[final]`,

		// Параметры
		width, height,
		width, height,
		avatarSize, avatarSize,
		avatarSize/2, avatarSize/2, avatarSize/2-6,
		avatarSize/2, avatarSize/2, avatarSize/2-6,
		avatarSize/2, avatarSize/2, avatarSize/2-6,
		avatarSize/2, avatarSize/2, avatarSize/2,
		avatarX,
		fontPath, displayUsername,
		waveformHeight/2,
		waveformHeight/2,
		width-100, waveformHeight/2,
		escapedSubtitlePath, fontName,
		fontPath,
	)

	// Собираем команду FFmpeg
	args := []string{
		"-i", audioPath,
		"-i", avatarPath,
		"-filter_complex", complexFilter,
		"-map", "[final]",
		"-map", "0:a",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-b:v", bitrate,
		"-r", strconv.Itoa(fps),
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Парсим прогресс из stderr
	// FFmpeg выводит прогресс с \r (carriage return), а не \n, поэтому читаем небольшими чанками
	progressRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

	lastPercent := 0
	buf := make([]byte, 256)
	var accumulated string

	// Получаем длительность аудио для корректного расчёта прогресса
	audioDuration := getAudioDuration(audioPath)
	if audioDuration <= 0 {
		audioDuration = 60 // fallback
	}

	go func() {
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				accumulated += string(buf[:n])

				// Ищем все совпадения времени в накопленных данных
				if matches := progressRegex.FindStringSubmatch(accumulated); matches != nil {
					hours, _ := strconv.Atoi(matches[1])
					minutes, _ := strconv.Atoi(matches[2])
					seconds, _ := strconv.Atoi(matches[3])
					totalSeconds := hours*3600 + minutes*60 + seconds

					// Рассчитываем процент на основе реальной длительности аудио
					percent := totalSeconds * 100 / int(audioDuration)
					if percent > 99 {
						percent = 99
					}

					if percent != lastPercent && onProgress != nil {
						onProgress(percent)
						lastPercent = percent
					}

					// Очищаем накопленные данные, оставляя только последние 100 символов
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

	// Удаляем файл субтитров
	os.Remove(subtitlePath)

	return outputPath, nil
}

// createSubtitleFile создаёт SRT файл из транскрипции
func createSubtitleFile(result *transcription.Result, outputPath string) error {
	words := result.Words
	if len(words) == 0 {
		text := result.Text
		if text == "" {
			text = "Без текста"
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

// formatSRTTime форматирует время в SRT формат
func formatSRTTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, ms)
}

// GenerateVerticalVideo создаёт вертикальное видео (9:16)
func GenerateVerticalVideo(audioPath, avatarPath string, transcriptionResult *transcription.Result, username string, fps int, bitrate string, onProgress ProgressCallback) (string, error) {
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	width := 720
	height := 1280

	outputPath := filepath.Join(tempDir, fmt.Sprintf("video_vert_%d.mp4", time.Now().UnixNano()))

	// Создаём SRT файл субтитров
	subtitlePath := filepath.Join(tempDir, fmt.Sprintf("subtitles_%d.srt", time.Now().UnixNano()))
	if err := createSubtitleFile(transcriptionResult, subtitlePath); err != nil {
		return "", fmt.Errorf("failed to create subtitles: %w", err)
	}

	// Определяем шрифт в зависимости от ОС
	var fontPath, fontName string
	if runtime.GOOS == "darwin" {
		fontPath = "/System/Library/Fonts/Helvetica.ttc"
		fontName = "Helvetica"
	} else {
		fontPath = "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf"
		fontName = "Noto Sans"
	}

	// Размеры элементов для вертикального видео
	avatarSize := 300
	avatarX := (width - avatarSize) / 2
	avatarY := 150
	waveformHeight := 200
	waveformY := 600

	// Форматируем юзернейм
	displayUsername := username
	if !strings.HasPrefix(displayUsername, "@") {
		displayUsername = "@" + displayUsername
	}

	// Экранируем путь к субтитрам
	escapedSubtitlePath := strings.ReplaceAll(subtitlePath, ":", "\\:")
	escapedSubtitlePath = strings.ReplaceAll(escapedSubtitlePath, "\\", "/")

	// Формируем complex filter для вертикального видео
	complexFilter := fmt.Sprintf(
		// 1. Фон
		`color=c=0x1a1a2e:s=%dx%d:d=10[bg];`+
			`[bg]drawbox=x=0:y=0:w=%d:h=%d:color=0x16213e:t=fill[bg_grad];`+

			// 2. Аватар - круглый с рамкой
			`[1:v]scale=%d:%d,format=rgba,`+
			`geq=r='if(gt(hypot(X-%d,Y-%d),%d),0,p(X,Y))':`+
			`g='if(gt(hypot(X-%d,Y-%d),%d),255,p(X,Y))':`+
			`b='if(gt(hypot(X-%d,Y-%d),%d),255,p(X,Y))':`+
			`a='if(gt(hypot(X-%d,Y-%d),%d),0,255)'[avatar_round];`+

			// Накладываем аватар
			`[bg_grad][avatar_round]overlay=%d:%d[with_avatar];`+

			// Добавляем юзернейм
			`[with_avatar]drawtext=fontfile=%s:text='%s':fontcolor=white:fontsize=48:x=(w-text_w)/2:y=%d[with_username];`+

			// 3. Визуализация
			`[0:a]volume=8.0[loud_a];`+
			`[loud_a]showfreqs=s=64x%d:mode=bar:ascale=log:fscale=log:colors=0x00ffff[freqs_base];`+
			`[freqs_base]format=yuv420p[freqs_base_yuv];`+
			`[freqs_base_yuv]scale=128:%d:flags=neighbor[freqs_x2];`+
			`[freqs_x2]geq=lum='if(mod(X,2),0,p(X,Y))':cb='if(mod(X,2),128,p(X,Y))':cr='if(mod(X,2),128,p(X,Y))'[freqs_holes];`+
			`[freqs_holes]scale=%d:%d:flags=neighbor[freqs_spaced];`+
			`[freqs_spaced]tmix=frames=3[freqs_smooth];`+
			`[freqs_smooth]split[freqs_top][freqs_mirror_src];`+
			`[freqs_mirror_src]vflip[freqs_bottom];`+
			`[freqs_top][freqs_bottom]vstack[freqs_full];`+
			`[freqs_full]colorkey=0x000000:0.01:0.1[freqs_transparent];`+
			`[with_username][freqs_transparent]overlay=50:%d[with_wave];`+

			// 4. Субтитры (внизу экрана, над логотипом бота)
			`[with_wave]subtitles=%s:force_style='FontName=%s,FontSize=24,PrimaryColour=&HFFFFFF&,Outline=0,Shadow=0,Alignment=2,MarginV=30,MarginL=30,MarginR=30'[with_subs];`+

			// 5. Имя бота
			`[with_subs]drawtext=fontfile=%s:text='@AudioVisualizerBot':fontcolor=0x00ffff:fontsize=32:x=(w-text_w)/2:y=h-80[final]`,

		// Параметры
		width, height,
		width, height,
		avatarSize, avatarSize,
		avatarSize/2, avatarSize/2, avatarSize/2-8,
		avatarSize/2, avatarSize/2, avatarSize/2-8,
		avatarSize/2, avatarSize/2, avatarSize/2-8,
		avatarSize/2, avatarSize/2, avatarSize/2,
		avatarX, avatarY,
		fontPath, displayUsername,
		avatarY+avatarSize+40,
		waveformHeight/2,
		waveformHeight/2,
		width-100, waveformHeight/2,
		waveformY,
		escapedSubtitlePath, fontName,
		fontPath,
	)

	// Собираем команду FFmpeg
	args := []string{
		"-i", audioPath,
		"-i", avatarPath,
		"-filter_complex", complexFilter,
		"-map", "[final]",
		"-map", "0:a",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-b:v", bitrate,
		"-r", strconv.Itoa(fps),
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Парсим прогресс из stderr
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

	// Удаляем файл субтитров
	os.Remove(subtitlePath)

	return outputPath, nil
}
