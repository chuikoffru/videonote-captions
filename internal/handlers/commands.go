package handlers

import (
	"context"
	"fmt"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// StartHandler обрабатывает команду /start
func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	// Регистрируем пользователя
	if h.DB != nil && update.Message.From != nil {
		_, err := h.DB.GetOrCreateUser(
			update.Message.From.ID,
			update.Message.From.Username,
			update.Message.From.FirstName,
			update.Message.From.LastName,
		)
		if err != nil {
			log.Printf("Failed to register user: %v", err)
		}
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "👋 Привет! Отправь мне голосовое сообщение или перешли чужое, и я создам видео с визуализацией аудио, аватаркой и субтитрами!",
	})
	if err != nil {
		log.Printf("Failed to send start message: %v", err)
	}
}

// StatsHandler обрабатывает команду /stats (только для админа)
func (h *Handler) StatsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	// Проверяем, что это админ
	if update.Message.From.ID != h.Config.AdminID {
		log.Printf("Stats requested by non-admin user: %d", update.Message.From.ID)
		return
	}

	if h.DB == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ База данных не инициализирована.",
		})
		return
	}

	stats, userCount, err := h.DB.GetStats()
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка получения статистики.",
		})
		return
	}

	statsText := fmt.Sprintf(`📊 *Статистика бота*

👥 Пользователей: %d
🎬 Бесплатных видео: %d
💳 Запросов оплаты: %d
✅ Оплаченных видео: %d
📱 Вертикальных видео: %d`,
		userCount,
		stats.TotalFreeVideos,
		stats.TotalPaymentRequests,
		stats.TotalPaidVideos,
		stats.TotalVerticalVideos,
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      statsText,
		ParseMode: models.ParseModeMarkdown,
	})
}
