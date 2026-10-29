package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) language(user *models.User) string {
	if user == nil {
		return "ru"
	}
	if h.DB != nil {
		if lang, err := h.DB.GetUserLanguage(user.ID); err == nil && (lang == "ru" || lang == "en") {
			return lang
		}
	}
	return fallbackLanguage(user.LanguageCode)
}
func fallbackLanguage(code string) string {
	if code == "" || strings.HasPrefix(strings.ToLower(code), "ru") {
		return "ru"
	}
	return "en"
}
func creditsWord(lang string, n int64) string {
	if lang == "en" {
		if n == 1 {
			return "credit"
		}
		return "credits"
	}
	return formatCreditsWord(n)
}
func formatDate(lang string, date time.Time) string {
	if lang == "en" {
		return date.Format("2 January 2006")
	}
	return date.Format("02.01.2006")
}

func (h *Handler) LanguageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil {
		return
	}
	msg := update.Message
	lang := h.language(msg.From)
	parts := strings.Fields(msg.Text)
	text := "Язык: /language ru или /language en"
	if lang == "en" {
		text = "Choose language: /language ru or /language en"
	}
	if len(parts) == 2 && (parts[1] == "ru" || parts[1] == "en") {
		if h.DB == nil {
			text = tr(lang, "❌ База данных не инициализирована.")
		} else {
			_, err := h.DB.GetOrCreateUser(msg.From.ID, msg.From.Username, msg.From.FirstName, msg.From.LastName)
			if err == nil {
				err = h.DB.SetUserLanguage(msg.From.ID, parts[1])
			}
			if err != nil {
				log.Printf("Set language: %v", err)
				text = tr(lang, "❌ База данных не инициализирована.")
			} else if parts[1] == "en" {
				text = "Language set to English."
			} else {
				text = "Выбран русский язык."
			}
		}
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: text})
}

func (h *Handler) SourcesHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil || h.Config == nil || update.Message.From.ID != h.Config.AdminID {
		return
	}
	lang := h.language(update.Message.From)
	text := tr(lang, "❌ База данных не инициализирована.")
	if h.DB != nil {
		stats, err := h.DB.GetSourceStats()
		if err != nil {
			log.Printf("Source stats: %v", err)
			text = tr(lang, "❌ Ошибка получения статистики.")
		} else {
			text = "📊 Источники первого запуска\nПользователи / с видео / с оплатой\n"
			if lang == "en" {
				text = "📊 First-start sources\nUsers / activated / paid users\n"
			}
			for _, row := range stats {
				text += fmt.Sprintf("\n%s: %d / %d / %d", row.Source, row.Users, row.Activated, row.PaidUsers)
			}
		}
	}
	// Keep every source while respecting Telegram's message size limit.
	chunk := ""
	for _, line := range strings.Split(text, "\n") {
		if utf8.RuneCountInString(chunk)+utf8.RuneCountInString(line)+1 > 3900 {
			if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: chunk}); err != nil {
				log.Printf("Send sources: %v", err)
				return
			}
			chunk = ""
		}
		chunk += line + "\n"
	}
	if chunk != "" {
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: chunk}); err != nil {
			log.Printf("Send sources: %v", err)
		}
	}
}
