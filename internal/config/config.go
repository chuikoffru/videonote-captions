package config

import (
	"os"
	"strconv"
)

type Config struct {
	BotToken     string
	OpenAIAPIKey string
	Proxy        string
	YuKassaToken string // provider_token для ЮКасса

	VideoWidth   int
	VideoHeight  int
	VideoFPS     int
	VideoBitrate string

	// Монетизация
	CreditsPerMinute int   // Стоимость каждых 60 секунд сверх бесплатного лимита (кредиты)
	FreeSeconds      int   // Бесплатный лимит в секундах
	AdminID          int64 // Telegram ID администратора (без лимитов)
	VideoNoteCredits       int // Стоимость обработки Video Note (кружочка)
	FreeVideoNotesPerMonth int // Бесплатных кружочков в месяц

	// Кредиты (цены в рублях)
	CreditPackage1Amount int // Количество кредитов в пакете 1
	CreditPackage1Price  int // Цена пакета 1 в рублях
	CreditPackage2Amount int // Количество кредитов в пакете 2
	CreditPackage2Price  int // Цена пакета 2 в рублях
	CreditPackage3Amount int // Количество кредитов в пакете 3
	CreditPackage3Price  int // Цена пакета 3 в рублях
}

func Load() *Config {
	cfg := &Config{
		BotToken:             os.Getenv("BOT_TOKEN"),
		OpenAIAPIKey:         os.Getenv("OPENAI_API_KEY"),
		Proxy:                os.Getenv("PROXY"),
		YuKassaToken:         os.Getenv("YUKASSA_PROVIDER_TOKEN"),
		VideoWidth:           1280,
		VideoHeight:          720,
		VideoFPS:             30,
		VideoBitrate:         "2000k",
		CreditsPerMinute:     10,   // По умолчанию 10 кредитов за 60 секунд
		FreeSeconds:          60,   // По умолчанию 60 секунд бесплатно
		CreditPackage1Amount: 100,  // 100 кредитов
		CreditPackage1Price:  99,   // 99 рублей
		CreditPackage2Amount: 300,  // 300 кредитов
		CreditPackage2Price:  249,  // 249 рублей
		CreditPackage3Amount: 1000, // 1000 кредитов
		CreditPackage3Price:  749,  // 749 рублей
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
	if fs, err := strconv.Atoi(os.Getenv("FREE_SECONDS")); err == nil && fs > 0 {
		cfg.FreeSeconds = fs
	}
	if cpm, err := strconv.Atoi(os.Getenv("CREDITS_PER_MINUTE")); err == nil && cpm > 0 {
		cfg.CreditsPerMinute = cpm
	}
	if adminID, err := strconv.ParseInt(os.Getenv("ADMIN_ID"), 10, 64); err == nil && adminID > 0 {
		cfg.AdminID = adminID
	}
	if vnc, err := strconv.Atoi(os.Getenv("VIDEO_NOTE_CREDITS")); err == nil && vnc > 0 {
		cfg.VideoNoteCredits = vnc
	} else {
		cfg.VideoNoteCredits = 10 // Дефолтное значение
	}
	if fvn, err := strconv.Atoi(os.Getenv("FREE_VIDEO_NOTES_PER_MONTH")); err == nil && fvn >= 0 {
		cfg.FreeVideoNotesPerMonth = fvn
	} else {
		cfg.FreeVideoNotesPerMonth = 3 // Дефолт: 3 бесплатных в месяц
	}

	// Загрузка настроек пакетов кредитов
	if cp1a, err := strconv.Atoi(os.Getenv("CREDIT_PACKAGE_1_AMOUNT")); err == nil && cp1a > 0 {
		cfg.CreditPackage1Amount = cp1a
	}
	if cp1p, err := strconv.Atoi(os.Getenv("CREDIT_PACKAGE_1_PRICE")); err == nil && cp1p > 0 {
		cfg.CreditPackage1Price = cp1p
	}
	if cp2a, err := strconv.Atoi(os.Getenv("CREDIT_PACKAGE_2_AMOUNT")); err == nil && cp2a > 0 {
		cfg.CreditPackage2Amount = cp2a
	}
	if cp2p, err := strconv.Atoi(os.Getenv("CREDIT_PACKAGE_2_PRICE")); err == nil && cp2p > 0 {
		cfg.CreditPackage2Price = cp2p
	}
	if cp3a, err := strconv.Atoi(os.Getenv("CREDIT_PACKAGE_3_AMOUNT")); err == nil && cp3a > 0 {
		cfg.CreditPackage3Amount = cp3a
	}
	if cp3p, err := strconv.Atoi(os.Getenv("CREDIT_PACKAGE_3_PRICE")); err == nil && cp3p > 0 {
		cfg.CreditPackage3Price = cp3p
	}

	return cfg
}
