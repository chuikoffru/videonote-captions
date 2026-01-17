package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// PendingCreditPurchases хранилище ожидающих покупок кредитов
var PendingCreditPurchases sync.Map

// CreditPurchaseData данные о покупке кредитов
type CreditPurchaseData struct {
	UserID    int64
	Amount    int // Количество кредитов
	PriceRub  int // Цена в рублях
	Timestamp int64
}

// BalanceHandler обрабатывает команду /balance
func (h *Handler) BalanceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	userID := update.Message.From.ID

	// Регистрируем пользователя если ещё не зарегистрирован
	if h.DB != nil {
		h.DB.GetOrCreateUser(userID, update.Message.From.Username, update.Message.From.FirstName, update.Message.From.LastName)
	}

	var credits int64
	var err error
	if h.DB != nil {
		credits, err = h.DB.GetUserCredits(userID)
		if err != nil {
			log.Printf("Ошибка получения баланса: %v", err)
			credits = 0
		}
	}

	// Форматируем слово "кредит" с правильным окончанием
	creditWord := formatCreditsWord(credits)

	text := fmt.Sprintf("💰 *Ваш баланс:* %d %s\n\n", credits, creditWord)
	text += "Используйте /buy для пополнения баланса"

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	})
}

// BuyHandler обрабатывает команду /buy - показывает пакеты кредитов
func (h *Handler) BuyHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	// Проверяем, настроен ли YuKassa
	if h.Config.YuKassaToken == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Оплата временно недоступна. Попробуйте позже.",
		})
		return
	}

	// Регистрируем пользователя
	if h.DB != nil {
		h.DB.GetOrCreateUser(update.Message.From.ID, update.Message.From.Username, update.Message.From.FirstName, update.Message.From.LastName)
	}

	// Получаем текущий баланс
	var credits int64
	if h.DB != nil {
		credits, _ = h.DB.GetUserCredits(update.Message.From.ID)
	}

	text := fmt.Sprintf("💰 *Ваш текущий баланс:* %d %s\n\n", credits, formatCreditsWord(credits))
	text += "🛒 *Выберите пакет кредитов:*"

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
		ParseMode:   models.ParseModeMarkdown,
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
	chatID := update.CallbackQuery.Message.Message.Chat.ID

	// Генерируем payload для идентификации платежа
	payloadID := fmt.Sprintf("credit:%d:%d:%d", userID, amount, time.Now().UnixNano())

	// Сохраняем данные платежа
	PendingCreditPurchases.Store(payloadID, CreditPurchaseData{
		UserID:    userID,
		Amount:    amount,
		PriceRub:  price,
		Timestamp: time.Now().Unix(),
	})

	log.Printf("Создаём invoice для покупки кредитов: user=%d, amount=%d, price=%d RUB", userID, amount, price)

	// Отправляем invoice
	_, err := b.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:        chatID,
		Title:         "Пополнение баланса",
		Description:   description,
		Payload:       payloadID,
		ProviderToken: h.Config.YuKassaToken,
		Currency:      "RUB",
		Prices: []models.LabeledPrice{
			{Label: description, Amount: price * 100}, // Цена в копейках
		},
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

	if _, ok := PendingCreditPurchases.Load(payload); !ok {
		log.Printf("CreditPreCheckoutQuery: payload not found")
		b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
			PreCheckoutQueryID: update.PreCheckoutQuery.ID,
			OK:                 false,
			ErrorMessage:       "❌ Заказ устарел. Пожалуйста, попробуйте снова через /buy",
		})
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

	value, ok := PendingCreditPurchases.Load(payload)
	if !ok {
		log.Printf("CreditSuccessfulPayment: payload not found")
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка: данные платежа не найдены. Обратитесь в поддержку.",
		})
		return
	}

	purchaseData := value.(CreditPurchaseData)
	PendingCreditPurchases.Delete(payload)

	// Начисляем кредиты
	if h.DB != nil {
		telegramPaymentChargeID := update.Message.SuccessfulPayment.TelegramPaymentChargeID
		description := fmt.Sprintf("Покупка %d кредитов за %d ₽", purchaseData.Amount, purchaseData.PriceRub)

		err := h.DB.AddCredits(purchaseData.UserID, int64(purchaseData.Amount), "purchase", description, telegramPaymentChargeID)
		if err != nil {
			log.Printf("Ошибка начисления кредитов: %v", err)
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "❌ Ошибка при начислении кредитов. Обратитесь в поддержку с ID платежа: " + telegramPaymentChargeID,
			})
			return
		}

		// Получаем новый баланс
		newBalance, _ := h.DB.GetUserCredits(purchaseData.UserID)

		log.Printf("Кредиты начислены: user=%d, amount=%d, new_balance=%d", purchaseData.UserID, purchaseData.Amount, newBalance)

		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      fmt.Sprintf("✅ *Баланс пополнен!*\n\n💰 Начислено: +%d %s\n💳 Ваш баланс: %d %s", purchaseData.Amount, formatCreditsWord(int64(purchaseData.Amount)), newBalance, formatCreditsWord(newBalance)),
			ParseMode: models.ParseModeMarkdown,
		})
	}
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
