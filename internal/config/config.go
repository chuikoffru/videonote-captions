package config

import (
	"os"
	"strconv"
)

type Config struct {
	BotToken     string
	OpenAIAPIKey string
	Proxy        string

	VideoWidth   int
	VideoHeight  int
	VideoFPS     int
	VideoBitrate string

	// Монетизация
	StarsPerMinute int   // Стоимость каждых 60 секунд сверх бесплатного лимита
	FreeSeconds    int   // Бесплатный лимит в секундах
	AdminID        int64 // Telegram ID администратора (без лимитов)
}

func Load() *Config {
	cfg := &Config{
		BotToken:       os.Getenv("BOT_TOKEN"),
		OpenAIAPIKey:   os.Getenv("OPENAI_API_KEY"),
		Proxy:          os.Getenv("PROXY"),
		VideoWidth:     1280,
		VideoHeight:    720,
		VideoFPS:       30,
		VideoBitrate:   "2000k",
		StarsPerMinute: 10, // По умолчанию 10 звёзд за 60 секунд
		FreeSeconds:    60, // По умолчанию 60 секунд бесплатно
	}

	if w, err := strconv.Atoi(os.Getenv("VIDEO_WIDTH")); err == nil && w > 0 {
		cfg.VideoWidth = w
	}
	if h, err := strconv.Atoi(os.Getenv("VIDEO_HEIGHT")); err == nil && h > 0 {
		cfg.VideoHeight = h
	}
	if fps, err := strconv.Atoi(os.Getenv("VIDEO_FPS")); err == nil && fps > 0 {
		cfg.VideoFPS = fps
	}
	if br := os.Getenv("VIDEO_BITRATE"); br != "" {
		cfg.VideoBitrate = br
	}
	if spm, err := strconv.Atoi(os.Getenv("STARS_PER_MINUTE")); err == nil && spm > 0 {
		cfg.StarsPerMinute = spm
	}
	if fs, err := strconv.Atoi(os.Getenv("FREE_SECONDS")); err == nil && fs > 0 {
		cfg.FreeSeconds = fs
	}
	if adminID, err := strconv.ParseInt(os.Getenv("ADMIN_ID"), 10, 64); err == nil && adminID > 0 {
		cfg.AdminID = adminID
	}

	return cfg
}
