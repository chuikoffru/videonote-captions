package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"videonote-captions/internal/database"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// sendInsufficientCreditsMessage отправляет сообщение о недостатке кредитов с кнопками покупки
func (h *Handler) sendInsufficientCreditsMessage(ctx context.Context, b *bot.Bot, chatID int64, needed int, balance int64) {
	text := fmt.Sprintf("❌ <b>Недостаточно кредитов</b>\n\n"+
		"💰 Нужно: %d %s\n"+
		"💳 У вас: %d %s\n\n"+
		"🛒 <b>Выберите пакет для пополнения:</b>",
		needed, formatCreditsWord(int64(needed)),
		balance, formatCreditsWord(balance))

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽", h.Config.CreditPackage1Amount, h.Config.CreditPackage1Price),
					CallbackData: "buy:1",
				},
			},
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽ (выгода!)", h.Config.CreditPackage2Amount, h.Config.CreditPackage2Price),
					CallbackData: "buy:2",
				},
			},
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽ (лучшая цена!)", h.Config.CreditPackage3Amount, h.Config.CreditPackage3Price),
					CallbackData: "buy:3",
				},
			},
		},
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}

// sendFreeNotesExhaustedMessage отправляет сообщение об исчерпании бесплатных кружочков
func (h *Handler) sendFreeNotesExhaustedMessage(ctx context.Context, b *bot.Bot, chatID int64, balance int64) {
	// Вычисляем дату следующего сброса (1 число следующего месяца)
	now := time.Now()
	nextReset := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())

	text := fmt.Sprintf("🎬 Бесплатные кружочки в этом месяце закончились!\n\n"+
		"🔄 Лимит сбросится: %s\n"+
		"💰 Стоимость 1 кружочка: %d %s\n"+
		"💳 Ваш баланс: %d %s\n\n"+
		"🛒 Выберите пакет для пополнения:",
		nextReset.Format("02.01.2006"),
		h.Config.VideoNoteCredits, formatCreditsWord(int64(h.Config.VideoNoteCredits)),
		balance, formatCreditsWord(balance))

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽", h.Config.CreditPackage1Amount, h.Config.CreditPackage1Price),
					CallbackData: "buy:1",
				},
			},
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽ (выгода!)", h.Config.CreditPackage2Amount, h.Config.CreditPackage2Price),
					CallbackData: "buy:2",
				},
			},
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽ (лучшая цена!)", h.Config.CreditPackage3Amount, h.Config.CreditPackage3Price),
					CallbackData: "buy:3",
				},
			},
		},
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
}

// BuyHandler обрабатывает команду /buy - показывает пакеты кредитов
func (h *Handler) BuyHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	userID := update.Message.From.ID

	// Регистрируем пользователя
	if h.DB != nil {
		h.DB.GetOrCreateUser(userID, update.Message.From.Username, update.Message.From.FirstName, update.Message.From.LastName)
	}

	// Получаем текущий баланс
	var credits int64
	if h.DB != nil {
		credits, _ = h.DB.GetUserCredits(userID)
	}

	text := fmt.Sprintf("💰 Ваш текущий баланс: %d %s\n\n", credits, formatCreditsWord(credits))

	// Добавляем информацию о бесплатных кружочках
	if h.DB != nil && h.Config.FreeVideoNotesPerMonth > 0 {
		freeRemaining, err := h.DB.GetFreeVideoNotesRemaining(userID, h.Config.FreeVideoNotesPerMonth)
		if err == nil {
			// Вычисляем количество использованных
			freeUsed := h.Config.FreeVideoNotesPerMonth - freeRemaining

			// Вычисляем дату следующего сброса (1 число следующего месяца)
			now := time.Now()
			nextReset := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())

			text += fmt.Sprintf("🎬 Использовано бесплатных кружочков: %d из %d\n", freeUsed, h.Config.FreeVideoNotesPerMonth)
			text += fmt.Sprintf("🔄 Обновление лимитов: %s\n\n", nextReset.Format("02.01.2006"))
		}
	}

	text += "🛒 Выберите пакет кредитов:"

	// Создаём кнопки с пакетами
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽", h.Config.CreditPackage1Amount, h.Config.CreditPackage1Price),
					CallbackData: "buy:1",
				},
			},
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽ (выгода!)", h.Config.CreditPackage2Amount, h.Config.CreditPackage2Price),
					CallbackData: "buy:2",
				},
			},
			{
				{
					Text:         fmt.Sprintf("🪙 %d кредитов — %d ₽ (лучшая цена!)", h.Config.CreditPackage3Amount, h.Config.CreditPackage3Price),
					CallbackData: "buy:3",
				},
			},
		},
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
}

// BuyCallbackHandler обрабатывает нажатие кнопки покупки пакета
func (h *Handler) BuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	// Отвечаем на callback чтобы убрать "часики"
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	data := update.CallbackQuery.Data
	if !strings.HasPrefix(data, "buy:") {
		return
	}

	packageNum := strings.TrimPrefix(data, "buy:")

	var amount, price int
	var description string

	switch packageNum {
	case "1":
		amount = h.Config.CreditPackage1Amount
		price = h.Config.CreditPackage1Price
		description = fmt.Sprintf("%d кредитов", amount)
	case "2":
		amount = h.Config.CreditPackage2Amount
		price = h.Config.CreditPackage2Price
		description = fmt.Sprintf("%d кредитов (выгода!)", amount)
	case "3":
		amount = h.Config.CreditPackage3Amount
		price = h.Config.CreditPackage3Price
		description = fmt.Sprintf("%d кредитов (лучшая цена!)", amount)
	default:
		return
	}

	userID := update.CallbackQuery.From.ID
	if update.CallbackQuery.Message.Message == nil {
		return
	}
	chatID := update.CallbackQuery.Message.Message.Chat.ID

	// Генерируем payload для идентификации платежа
	payloadID := fmt.Sprintf("credit:%d:%d:%d", userID, amount, time.Now().UnixNano())

	// Сохраняем данные платежа
	if h.DB == nil || h.Config.YuKassaToken == "" || update.CallbackQuery.Message.Message == nil {
		return
	}
	if _, err := h.DB.GetOrCreateUser(userID, update.CallbackQuery.From.Username, update.CallbackQuery.From.FirstName, update.CallbackQuery.From.LastName); err != nil {
		log.Printf("Register buyer: %v", err)
		return
	}
	if err := h.DB.CreateOrder(database.PaymentOrder{Payload: payloadID, UserID: userID, Amount: amount, PriceRub: price}); err != nil {
		log.Printf("Create order: %v", err)
		return
	}

	log.Printf("Создаём invoice для покупки кредитов: user=%d, amount=%d, price=%d RUB", userID, amount, price)

	// Формируем provider_data для чека ЮKassa
	// Сумма в provider_data указывается в рублях (не в копейках!)
	providerData := map[string]interface{}{
		"receipt": map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"description": description,
					"quantity":    1,
					"amount": map[string]interface{}{
						"value":    float64(price), // В рублях
						"currency": "RUB",
					},
					"vat_code":        1, // НДС не облагается
					"payment_mode":    "full_payment",
					"payment_subject": "service", // Услуга
				},
			},
			"tax_system_code": 2, // УСН (доходы)
		},
	}

	providerDataJSON, err := json.Marshal(providerData)
	if err != nil {
		log.Printf("Ошибка сериализации provider_data: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка при создании платежа. Попробуйте позже.",
		})
		return
	}

	log.Printf("provider_data: %s", string(providerDataJSON))

	// Отправляем invoice с параметрами для фискализации
	_, err = b.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:        chatID,
		Title:         "Пополнение баланса",
		Description:   description,
		Payload:       payloadID,
		ProviderToken: h.Config.YuKassaToken,
		Currency:      "RUB",
		Prices: []models.LabeledPrice{
			{Label: description, Amount: price * 100}, // Цена в копейках
		},
		NeedEmail:           true, // Запрашиваем email у пользователя
		SendEmailToProvider: true, // Отправляем email провайдеру для чека
		ProviderData:        string(providerDataJSON),
	})

	if err != nil {
		log.Printf("Ошибка отправки invoice: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка при создании платежа. Попробуйте позже.",
		})
	}
}

// CreditPreCheckoutHandler обрабатывает pre_checkout_query для покупки кредитов (RUB)
func (h *Handler) CreditPreCheckoutHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.PreCheckoutQuery == nil {
		return
	}

	payload := update.PreCheckoutQuery.InvoicePayload
	log.Printf("CreditPreCheckoutQuery received, payload: %s", payload)

	// Проверяем, что это покупка кредитов
	if !strings.HasPrefix(payload, "credit:") {
		log.Printf("CreditPreCheckoutQuery: not a credit purchase, skipping")
		return
	}

	p, err := h.DB.GetOrder(payload)
	q := update.PreCheckoutQuery
	if err != nil || p.Paid || p.UserID != q.From.ID || p.PriceRub*100 != q.TotalAmount || q.Currency != "RUB" {
		b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{PreCheckoutQueryID: q.ID, OK: false, ErrorMessage: "Заказ недействителен. Используйте /buy"})
		return
	}

	log.Printf("CreditPreCheckoutQuery: payload found, answering OK")
	b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
}

// CreditSuccessfulPaymentHandler обрабатывает успешный платёж за кредиты (RUB)
func (h *Handler) CreditSuccessfulPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Printf("CreditSuccessfulPaymentHandler called")

	if update.Message == nil || update.Message.SuccessfulPayment == nil {
		log.Printf("CreditSuccessfulPaymentHandler: no SuccessfulPayment in message")
		return
	}

	payload := update.Message.SuccessfulPayment.InvoicePayload
	log.Printf("CreditSuccessfulPayment received, payload: %s", payload)

	// Проверяем, что это покупка кредитов
	if !strings.HasPrefix(payload, "credit:") {
		log.Printf("CreditSuccessfulPayment: not a credit purchase, skipping")
		return
	}

	payment := update.Message.SuccessfulPayment
	if update.Message.From == nil {
		return
	}
	userID := update.Message.From.ID
	if err := h.DB.CompleteOrder(payload, userID, payment.TotalAmount, payment.Currency, payment.TelegramPaymentChargeID); err != nil {
		log.Printf("Complete payment: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "Не удалось зачислить платёж. Обратитесь в поддержку с ID: " + payment.TelegramPaymentChargeID})
		return
	}
	balance, _ := h.DB.GetUserCredits(userID)
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: fmt.Sprintf("✅ Платёж учтён. Баланс: %d кредитов", balance)})
}

// formatCreditsWord возвращает правильное склонение слова "кредит"
func formatCreditsWord(n int64) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}

	lastTwo := abs % 100
	lastOne := abs % 10

	if lastTwo >= 11 && lastTwo <= 19 {
		return "кредитов"
	}

	switch lastOne {
	case 1:
		return "кредит"
	case 2, 3, 4:
		return "кредита"
	default:
		return "кредитов"
	}
}
