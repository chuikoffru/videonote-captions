package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"

	"telegram-voice-video-bot/internal/config"
	"telegram-voice-video-bot/internal/database"
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

	// Инициализируем базу данных
	db, err := database.New("data/bot.db")
	if err != nil {
		log.Printf("Warning: Failed to initialize database: %v", err)
		db = nil // Бот будет работать без базы данных
	}

	// Создаём контекст с обработкой сигналов
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Создаём глобальный обработчик
	h := handlers.NewHandler(cfg, db)

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
			if update.CallbackQuery != nil {
				log.Printf("Update: CallbackQuery received, data: %s", update.CallbackQuery.Data)
			}

			// Обрабатываем команду /start
			if update.Message != nil && update.Message.Text == "/start" {
				h.StartHandler(ctx, b, update)
				return
			}

			// Обрабатываем команду /stats (только для админа)
			if update.Message != nil && strings.HasPrefix(update.Message.Text, "/stats") {
				h.StatsHandler(ctx, b, update)
				return
			}

			// Обрабатываем команду /broadcast (только для админа)
			if update.Message != nil && strings.HasPrefix(update.Message.Text, "/broadcast") {
				h.BroadcastHandler(ctx, b, update)
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

			// Обрабатываем нажатия на кнопки (CallbackQuery)
			if update.CallbackQuery != nil {
				if strings.HasPrefix(update.CallbackQuery.Data, "vertcache:") {
					h.VerticalCacheCallbackHandler(ctx, b, update)
				} else {
					h.VerticalCallbackHandler(ctx, b, update)
				}
				return
			}
		}),
		bot.WithAllowedUpdates(bot.AllowedUpdates{
			"message",
			"callback_query",
			"pre_checkout_query",
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
