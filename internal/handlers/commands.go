package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"videonote-captions/internal/attribution"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// StartHandler обрабатывает команду /start
func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}

	lang := h.language(update.Message.From)

	source, hint, valid := attribution.ParseStart(update.Message.Text)
	if hint != "" {
		lang = hint
	}
	if h.DB != nil && update.Message.From != nil {
		user := update.Message.From
		if _, err := h.DB.GetOrCreateUser(user.ID, user.Username, user.FirstName, user.LastName); err != nil {
			log.Printf("Register user: %v", err)
		} else if valid {
			if err := h.DB.RecordStart(user.ID, source, lang); err != nil {
				log.Printf("Record start: %v", err)
			}
			if hint != "" {
				if err := h.DB.SetUserLanguage(user.ID, hint); err != nil {
					log.Printf("Set start language: %v", err)
				}
			}
		}
	}

	welcome := tr(lang, "👋 Отправь кружочек — я добавлю субтитры: одно слово в момент его произнесения. /help — условия, /buy — баланс и кредиты.")
	if lang == "ru" {
		welcome += " /language ru|en — язык."
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   welcome,
	})
	if err != nil {
		log.Printf("Failed to send start message: %v", err)
	}
}

// HelpHandler обрабатывает команду /help
func (h *Handler) HelpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}

	lang := h.language(update.Message.From)
	helpText := fmt.Sprintf(tr(lang, "Отправь или перешли видеокружочек. Я верну его с субтитрами — по одному слову, синхронно с речью — и полным текстом под ним.\n\nБесплатно: %d кружочка в месяц. Далее: %d кредитов за кружочек. /buy — баланс и пополнение.\n\nЗвук отправляется в OpenAI для распознавания речи."), h.Config.FreeVideoNotesPerMonth, h.Config.VideoNoteCredits)

	if lang == "ru" {
		helpText += "\n\nСубтитры остаются на языке речи. /language ru|en — язык интерфейса."
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      helpText,
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		log.Printf("Failed to send help message: %v", err)
	}
}

// BroadcastHandler обрабатывает команду /broadcast (только для админа)
func (h *Handler) BroadcastHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil {
		return
	}

	lang := h.language(update.Message.From)
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
				Text:      tr(lang, "ℹ️ Используйте команду так:\n`/broadcast Текст сообщения`\nили ответьте на сообщение командой `/broadcast`"),
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
	if h.DB == nil {
		return
	}
	users, err := h.DB.GetAllUsers()
	if err != nil {
		log.Printf("Failed to get users for broadcast: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   tr(lang, "❌ Ошибка получения списка пользователей."),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf(tr(lang, "🚀 Начинаю рассылку для %d пользователей..."), len(users)),
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
		Text:   fmt.Sprintf(tr(lang, "✅ Рассылка завершена!\n\nУспешно: %d\nОшибок: %d"), success, failed),
	})
}

// UsageHandler обрабатывает команду /usage (только для админа)
// Объединяет функционал бывшей команды /stats
func (h *Handler) UsageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil {
		return
	}

	lang := h.language(update.Message.From)
	// Проверяем, что это админ
	if update.Message.From.ID != h.Config.AdminID {
		return
	}

	if h.DB == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   tr(lang, "❌ База данных не инициализирована."),
		})
		return
	}

	stats, err := h.DB.GetFullStats()
	if err != nil {
		log.Printf("Failed to get full stats: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   tr(lang, "❌ Ошибка получения статистики."),
		})
		return
	}

	usageText := fmt.Sprintf(tr(lang, "📊 Статистика\nПользователей: %d\nБез видео: %d\nОдно видео: %d\nБольше одного: %d\nКружочков: %d\nБесплатных: %d\nПлатных: %d\nСумма балансов: %d кредитов\nПотрачено: %d кредитов"), stats.TotalUsers, stats.UsersNoVideos, stats.UsersOneVideo, stats.UsersMoreVideos, stats.VideoNotes, stats.FreeVideos, stats.PaidVideos, stats.TotalBalances, stats.TotalSpent)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      usageText,
		ParseMode: models.ParseModeMarkdown,
	})
}
