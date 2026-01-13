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
}

func Load() *Config {
	cfg := &Config{
		BotToken:     os.Getenv("BOT_TOKEN"),
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		Proxy:        os.Getenv("PROXY"),
		VideoWidth:   1280,
		VideoHeight:  720,
		VideoFPS:     30,
		VideoBitrate: "2000k",
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

	return cfg
}
