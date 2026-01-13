package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"telegram-voice-video-bot/internal/config"
	"telegram-voice-video-bot/internal/handlers"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	// Загружаем конфигурацию
	cfg := config.Load()

	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	// Создаём контекст с обработкой сигналов
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Создаём обработчики
	h := handlers.NewHandler(cfg)

	// Создаём бота
	opts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Регистрируем обработчики
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, h.StartHandler)

	// Обработчик голосовых сообщений
	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message != nil && update.Message.Voice != nil {
			h.VoiceHandler(ctx, b, update)
		}
	})

	// Запускаем бота
	log.Println("✅ Бот запущен!")
	b.Start(ctx)
}

func defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Создаём обработчик с конфигурацией
	cfg := config.Load()
	h := handlers.NewHandler(cfg)

	// Логируем входящие обновления для отладки
	if update.Message != nil {
		log.Printf("defaultHandler: Message received, has Voice: %v, has SuccessfulPayment: %v",
			update.Message.Voice != nil, update.Message.SuccessfulPayment != nil)
	}
	if update.PreCheckoutQuery != nil {
		log.Printf("defaultHandler: PreCheckoutQuery received")
	}

	// Обрабатываем разные типы обновлений
	if update.Message != nil {
		if update.Message.Voice != nil {
			h.VoiceHandler(ctx, b, update)
			return
		}
		if update.Message.SuccessfulPayment != nil {
			h.SuccessfulPaymentHandler(ctx, b, update)
			return
		}
	}

	if update.PreCheckoutQuery != nil {
		h.PreCheckoutHandler(ctx, b, update)
		return
	}
}
