package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"videonote-captions/internal/config"
	"videonote-captions/internal/database"
	"videonote-captions/internal/handlers"
	"videonote-captions/internal/httpclient"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	// Загружаем конфигурацию
	cfg := config.Load()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Fatal(err)
	}
	time.Local = loc

	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	if cfg.OpenAIAPIKey == "" || cfg.DatabaseURL == "" {
		log.Fatal("OPENAI_API_KEY and DATABASE_URL are required")
	}

	// Настраиваем общий HTTP-клиент для скачивания файлов Telegram через прокси
	if err := httpclient.Init(cfg.Proxy); err != nil {
		log.Fatalf("Invalid PROXY url: %v", err)
	}

	// Инициализируем базу данных
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize Postgres: %v", err)
	}

	// Создаём контекст с обработкой сигналов
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Создаём глобальный обработчик
	h := handlers.NewHandler(cfg, db)

	// Создаём бота с defaultHandler который обрабатывает ВСЕ типы обновлений
	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {

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

			if update.Message != nil && update.Message.VideoNote != nil {
				h.VideoNoteHandler(ctx, b, update)
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

				return
			}
			if update.Message != nil {
				h.HelpHandler(ctx, b, update)
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
				// Отключаем HTTP/2 к прокси: многие HTTPS-прокси не умеют
				// CONNECT поверх HTTP/2 и рвут соединение ("unexpected EOF").
				TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
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
