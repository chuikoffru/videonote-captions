package handlers

import (
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// PreCheckoutHandler обрабатывает pre_checkout_query
func (h *Handler) PreCheckoutHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.PreCheckoutQuery == nil {
		return
	}

	payload := update.PreCheckoutQuery.InvoicePayload
	log.Printf("PreCheckoutQuery received, payload: %s", payload)

	if _, ok := PendingInvoices.Load(payload); !ok {
		log.Printf("PreCheckoutQuery: payload not found in PendingInvoices")
		b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
			PreCheckoutQueryID: update.PreCheckoutQuery.ID,
			OK:                 false,
			ErrorMessage:       "❌ Заказ устарел или бот был перезагружен. Пожалуйста, отправьте сообщение снова.",
		})
		return
	}

	log.Printf("PreCheckoutQuery: payload found, answering OK")
	b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
}

// SuccessfulPaymentHandler обрабатывает успешный платёж
func (h *Handler) SuccessfulPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Printf("SuccessfulPaymentHandler called")

	if update.Message == nil || update.Message.SuccessfulPayment == nil {
		log.Printf("SuccessfulPaymentHandler: no SuccessfulPayment in message")
		return
	}

	payload := update.Message.SuccessfulPayment.InvoicePayload
	log.Printf("SuccessfulPayment received, payload: %s", payload)

	value, ok := PendingInvoices.Load(payload)
	if !ok {
		log.Printf("SuccessfulPayment: payload not found in PendingInvoices")
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка: заказ не найден (возможно, бот был перезагружен). Повторите попытку.",
		})
		return
	}

	log.Printf("SuccessfulPayment: payload found, starting processing")
	orderData := value.(OrderData)
	PendingInvoices.Delete(payload)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Оплата прошла успешно! Начинаю обработку.",
	})

	if orderData.Type == "vertical" {
		h.handleVerticalVideoRequest(ctx, b, update.Message, orderData.FileID, orderData.FileUniqueID, &orderData.Author)
	} else {
		voice := &models.Voice{
			FileID:       orderData.FileID,
			FileUniqueID: orderData.FileUniqueID,
			Duration:     0,
		}
		h.handleVoiceMessage(ctx, b, update.Message, voice, true, &orderData.Author, orderData.FileUniqueID)
	}
}
