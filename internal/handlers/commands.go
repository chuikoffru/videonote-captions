package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

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

// HelpHandler обрабатывает команду /help
func (h *Handler) HelpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	helpText := `🤖 <b>Как пользоваться ботом:</b>

1️⃣ <b>Отправь голосовое сообщение</b> (или перешли его от другого пользователя/канала).
2️⃣ Бот создаст видео с визуализацией звука, аватаркой и субтитрами.
3️⃣ <b>Первые 60 секунд</b> обработки бесплатно для каждого сообщения.
4️⃣ Если сообщение длиннее, можно оплатить продолжение за Telegram Stars (10 звезд за минуту).

📹 <b>Дополнительно:</b>
• Бот умеет создавать <b>вертикальные видео</b> (особенно круто для Stories/Reels/Shorts).
• Нажми кнопку "📱 Сделать вертикально" под готовым видео, чтобы получить вертикальную версию бесплатно (если она закеширована) или сгенерировать новую.

🚀 <b>Совет:</b> Пересылай голосовые сообщения из других чатов — бот возьмет аватарку и имя оригинального автора!`

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      helpText,
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		log.Printf("Failed to send help message: %v", err)
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

// BroadcastHandler обрабатывает команду /broadcast (только для админа)
func (h *Handler) BroadcastHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	// Проверяем, что это админ
	if update.Message.From.ID != h.Config.AdminID {
		return
	}

	// Определяем сообщение для рассылки
	var msgToSend *models.Message
	if update.Message.ReplyToMessage != nil {
		msgToSend = update.Message.ReplyToMessage
	} else {
		// Если это просто команда /broadcast текст
		parts := strings.SplitN(update.Message.Text, " ", 2)
		if len(parts) < 2 {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    update.Message.Chat.ID,
				Text:      "ℹ️ Используйте команду так:\n`/broadcast Текст сообщения`\nили ответьте на сообщение командой `/broadcast`",
				ParseMode: models.ParseModeMarkdown,
			})
			return
		}
		// Создаем фейковое сообщение с текстом для отправки
		msgToSend = &models.Message{
			Text: parts[1],
		}
	}

	// Получаем список всех пользователей
	users, err := h.DB.GetAllUsers()
	if err != nil {
		log.Printf("Failed to get users for broadcast: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка получения списка пользователей.",
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("🚀 Начинаю рассылку для %d пользователей...", len(users)),
	})

	success := 0
	failed := 0

	for _, userID := range users {
		var err error
		if update.Message.ReplyToMessage != nil {
			// Если это реплай - пересылаем (копируем) сообщение
			_, err = b.CopyMessage(ctx, &bot.CopyMessageParams{
				ChatID:     userID,
				FromChatID: update.Message.Chat.ID,
				MessageID:  msgToSend.ID,
			})
		} else {
			// Если просто текст
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: userID,
				Text:   msgToSend.Text,
			})
		}

		if err != nil {
			log.Printf("Failed to send broadcast to %d: %v", userID, err)
			failed++
		} else {
			success++
		}

		// Пауза чтобы не ловить лимиты слишком жестко, хотя CopyMessage быстр
		// time.Sleep(time.Millisecond * 30)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("✅ Рассылка завершена!\n\nУспешно: %d\nОшибок: %d", success, failed),
	})
}

// UsageHandler обрабатывает команду /usage (только для админа)
func (h *Handler) UsageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	// Проверяем, что это админ
	if update.Message.From.ID != h.Config.AdminID {
		return
	}

	if h.DB == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ База данных не инициализирована.",
		})
		return
	}

	users0, users1, usersMore, err := h.DB.GetUsageStats()
	if err != nil {
		log.Printf("Failed to get usage stats: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка получения статистики использования.",
		})
		return
	}

	usageText := fmt.Sprintf(`📊 *Статистика использования*

0️⃣ Ни одного видео: %d пользователей
1️⃣ Только 1 видео: %d пользователей
🚀 Больше 1 видео: %d пользователей`,
		users0,
		users1,
		usersMore,
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      usageText,
		ParseMode: models.ParseModeMarkdown,
	})
}
