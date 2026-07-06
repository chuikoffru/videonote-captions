package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"telegram-voice-video-bot/internal/config"
	"telegram-voice-video-bot/internal/database"
	"telegram-voice-video-bot/internal/handlers"
	"telegram-voice-video-bot/internal/httpclient"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	// Загружаем конфигурацию
	cfg := config.Load()

	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	// Настраиваем общий HTTP-клиент для скачивания файлов Telegram через прокси
	if err := httpclient.Init(cfg.Proxy); err != nil {
		log.Fatalf("Invalid PROXY url: %v", err)
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

			// Обрабатываем команду /help
			if update.Message != nil && update.Message.Text == "/help" {
				h.HelpHandler(ctx, b, update)
				return
			}

			// Обрабатываем команду /broadcast (только для админа)
			if update.Message != nil && strings.HasPrefix(update.Message.Text, "/broadcast") {
				h.BroadcastHandler(ctx, b, update)
				return
			}

			// Обрабатываем команду /usage (только для админа)
			if update.Message != nil && strings.HasPrefix(update.Message.Text, "/usage") {
				h.UsageHandler(ctx, b, update)
				return
			}

			// Обрабатываем команду /buy
			if update.Message != nil && strings.HasPrefix(update.Message.Text, "/buy") {
				h.BuyHandler(ctx, b, update)
				return
			}

			// Обрабатываем фото (кастомные фоны)
			if update.Message != nil && update.Message.Photo != nil {
				h.PhotoHandler(ctx, b, update)
				return
			}

			// Обрабатываем Video Note (кружочки)
			if update.Message != nil && update.Message.VideoNote != nil {
				h.VideoNoteHandler(ctx, b, update)
				return
			}

			// Обрабатываем голосовые сообщения
			if update.Message != nil && update.Message.Voice != nil {
				h.VoiceHandler(ctx, b, update)
				return
			}

			// Обрабатываем успешный платёж (только кредиты через ЮКасса)
			if update.Message != nil && update.Message.SuccessfulPayment != nil {
				h.CreditSuccessfulPaymentHandler(ctx, b, update)
				return
			}

			// Обрабатываем pre_checkout_query (только кредиты)
			if update.PreCheckoutQuery != nil {
				h.CreditPreCheckoutHandler(ctx, b, update)
				return
			}

			// Обрабатываем нажатия на кнопки (CallbackQuery)
			if update.CallbackQuery != nil {
				if strings.HasPrefix(update.CallbackQuery.Data, "buy:") {
					h.BuyCallbackHandler(ctx, b, update)
					return
				}
				if strings.HasPrefix(update.CallbackQuery.Data, "horiz:") {
					h.HorizontalCallbackHandler(ctx, b, update)
					return
				}
				if strings.HasPrefix(update.CallbackQuery.Data, "vertcache:") {
					h.VerticalCacheCallbackHandler(ctx, b, update)
				} else if strings.HasPrefix(update.CallbackQuery.Data, "vert:") {
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

	// Если задан PROXY — направляем запросы к Telegram API через прокси
	// (api.telegram.org может быть недоступен напрямую с сервера).
	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			log.Fatalf("Invalid PROXY url: %v", err)
		}
		// pollTimeout по умолчанию 60s (long polling getUpdates);
		// таймаут клиента ставим с запасом, чтобы не обрывать опрос.
		pollTimeout := time.Minute
		httpClient := &http.Client{
			Timeout: pollTimeout + 30*time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
		opts = append(opts, bot.WithHTTPClient(pollTimeout, httpClient))
		log.Println("🌐 Telegram API через прокси включён")
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Запускаем бота
	log.Println("✅ Бот запущен!")
	b.Start(ctx)
}
