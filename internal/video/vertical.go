package video

import (
	"fmt"
	"os"

	"telegram-voice-video-bot/internal/transcription"
)

// GenerateVerticalVideo создаёт вертикальное видео (9:16) с визуализацией аудио, аватаркой и субтитрами
func GenerateVerticalVideo(audioPath, avatarPath, backgroundPath string, transcriptionResult *transcription.Result, username string, fps int, bitrate string, onProgress ProgressCallback) (string, error) {
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	width := 720
	height := 1280

	// Подготавливаем субтитры
	subtitlePath, escapedSubtitlePath, err := prepareSubtitles(transcriptionResult, tempDir)
	if err != nil {
		return "", err
	}
	defer os.Remove(subtitlePath)

	// Строим фильтр
	complexFilter := buildVerticalFilter(width, height, escapedSubtitlePath, username, backgroundPath != "")

	// Генерируем видео
	cfg := VideoConfig{
		AudioPath:      audioPath,
		AvatarPath:     avatarPath,
		BackgroundPath: backgroundPath,
		Transcription:  transcriptionResult,
		Username:       username,
		Width:          width,
		Height:         height,
		FPS:            fps,
		Bitrate:        bitrate,
		OnProgress:     onProgress,
		OutputPrefix:   "video_vert",
	}

	return generateVideoWithFilter(cfg, complexFilter)
}

// buildVerticalFilter строит complex filter для вертикального видео
func buildVerticalFilter(width, height int, escapedSubtitlePath, username string, hasBackground bool) string {
	font := getFontConfig()
	displayUsername := formatUsername(username)

	avatarSize := 300
	avatarX := (width - avatarSize) / 2
	avatarY := 150
	waveformHeight := 200
	waveformY := 600

	// Выбираем источник фона
	var bgSource string
	if hasBackground {
		// Используем третий input как фон, масштабируем под размер видео
		bgSource = fmt.Sprintf(`[2:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d[bg_grad];`, width, height, width, height)
	} else {
		// Дефолтный тёмный фон
		bgSource = fmt.Sprintf(`color=c=0x1a1a2e:s=%dx%d:d=10[bg];`+
			`[bg]drawbox=x=0:y=0:w=%d:h=%d:color=0x16213e:t=fill[bg_grad];`, width, height, width, height)
	}

	return bgSource + fmt.Sprintf(
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
		avatarSize, avatarSize,
		avatarSize/2, avatarSize/2, avatarSize/2-8,
		avatarSize/2, avatarSize/2, avatarSize/2-8,
		avatarSize/2, avatarSize/2, avatarSize/2-8,
		avatarSize/2, avatarSize/2, avatarSize/2,
		avatarX, avatarY,
		font.Path, displayUsername,
		avatarY+avatarSize+40,
		waveformHeight/2,
		waveformHeight/2,
		width-100, waveformHeight/2,
		waveformY,
		escapedSubtitlePath, font.Name,
		font.Path,
	)
}
