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

	// Создаём глобальный обработчик
	h := handlers.NewHandler(cfg)

	// Создаём бота с defaultHandler который обрабатывает ВСЕ типы обновлений
	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
			// Логируем входящие обновления для отладки
			if update.Message != nil {
				log.Printf("Update: Message received, has Voice: %v, has SuccessfulPayment: %v, Text: %s",
					update.Message.Voice != nil, update.Message.SuccessfulPayment != nil, update.Message.Text)
			}
			if update.PreCheckoutQuery != nil {
				log.Printf("Update: PreCheckoutQuery received")
			}

			// Обрабатываем команду /start
			if update.Message != nil && update.Message.Text == "/start" {
				h.StartHandler(ctx, b, update)
				return
			}

			// Обрабатываем голосовые сообщения
			if update.Message != nil && update.Message.Voice != nil {
				h.VoiceHandler(ctx, b, update)
				return
			}

			// Обрабатываем успешный платёж
			if update.Message != nil && update.Message.SuccessfulPayment != nil {
				h.SuccessfulPaymentHandler(ctx, b, update)
				return
			}

			// Обрабатываем pre_checkout_query
			if update.PreCheckoutQuery != nil {
				h.PreCheckoutHandler(ctx, b, update)
				return
			}
		}),
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Запускаем бота
	log.Println("✅ Бот запущен!")
	b.Start(ctx)
}
