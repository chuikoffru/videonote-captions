package handlers

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"path/filepath"
	"strings"
)

func AcceptsVideo(msg *models.Message) bool {
	if msg == nil {
		return false
	}
	if msg.Video != nil {
		return true
	}
	if msg.Document == nil {
		return false
	}
	if strings.HasPrefix(msg.Document.MimeType, "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(msg.Document.FileName)) {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v":
		return true
	}
	return false
}

func (h *Handler) VideoHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil || h.Config == nil || !h.Config.EnableVideoToCircle || !AcceptsVideo(update.Message) {
		return
	}
	msg := update.Message
	lang := h.language(msg.From)
	say := func(text string) {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: text, ReplyParameters: &models.ReplyParameters{MessageID: msg.ID}})
	}
	if msg.Chat.Type != models.ChatTypePrivate {
		say(tr(lang, "Пришлите видео в личный чат с ботом."))
		return
	}
	fileID := ""
	var size int64
	duration := 0
	if msg.Video != nil {
		fileID = msg.Video.FileID
		size = msg.Video.FileSize
		duration = msg.Video.Duration
	} else {
		fileID = msg.Document.FileID
		size = msg.Document.FileSize
	}
	if fileID == "" {
		return
	}
	if size > h.Config.MaxInputVideoBytes {
		say(h.inputLimits(lang))
		return
	}
	if duration > h.Config.MaxInputVideoSeconds {
		say(h.inputLimits(lang))
		return
	}
	if h.DB != nil {
		if _, err := h.DB.GetOrCreateUser(msg.From.ID, msg.From.Username, msg.From.FirstName, msg.From.LastName); err != nil {
			say(tr(lang, "Сервис временно недоступен. Попробуйте позже."))
			return
		}
	}
	req := ProcessingRequest{OwnerID: msg.From.ID, ChatID: msg.Chat.ID, SourceMessageID: msg.ID, FileID: fileID, SourceKind: "video", Language: lang}
	token, err := h.pending.Put(req, duration)
	if err != nil {
		say(tr(lang, "Слишком много открытых меню. Завершите или отмените предыдущие."))
		return
	}
	text := fmt.Sprintf(tr(lang, "Выберите результат. Обрежу центр кадра в квадрат.\n\nБез субтитров: бесплатно, до %d кружочков в день (Москва).\nС субтитрами: %d бесплатно в месяц, далее %d кредитов.\nМеню действует 30 минут."), h.Config.PlainCirclesPerDay, h.Config.FreeVideoNotesPerMonth, h.Config.VideoNoteCredits)
	if duration > 60 {
		text += "\n\n" + tr(lang, "Видео длиннее минуты. Перед обрезкой первых 60 секунд попрошу подтверждение.")
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: text, ReplyParameters: &models.ReplyParameters{MessageID: msg.ID}, ReplyMarkup: circleButtons(lang, token, false)})
}

func circleButtons(lang, token string, plainOnly bool) *models.InlineKeyboardMarkup {
	rows := [][]models.InlineKeyboardButton{{{Text: tr(lang, "⭕ Сделать кружочек"), CallbackData: "circle:" + token + ":plain"}}}
	if !plainOnly {
		rows = append(rows, []models.InlineKeyboardButton{{Text: tr(lang, "💬 Кружочек с субтитрами"), CallbackData: "circle:" + token + ":captions"}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: tr(lang, "Отмена"), CallbackData: "circle:" + token + ":cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func trimButtons(lang, token string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: tr(lang, "Использовать первые 60 секунд"), CallbackData: "circle:" + token + ":trim"}}, {{Text: tr(lang, "Отмена"), CallbackData: "circle:" + token + ":cancel"}},
	}}
}

func (h *Handler) CircleCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.CallbackQuery == nil {
		return
	}
	q := update.CallbackQuery
	lang := h.language(&q.From)
	answer := func(text string) {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID, Text: text})
	}
	if h.Config == nil || !h.Config.EnableVideoToCircle || q.Message.Message == nil || q.Message.Message.Chat.Type != models.ChatTypePrivate {
		answer(tr(lang, "Меню недоступно. Пришлите видео заново."))
		return
	}
	parts := strings.Split(q.Data, ":")
	if len(parts) != 3 || parts[0] != "circle" {
		answer(tr(lang, "Меню недоступно. Пришлите видео заново."))
		return
	}
	msg := q.Message.Message
	p, err := h.pending.Select(parts[1], q.From.ID, msg.Chat.ID, parts[2])
	if err != nil {
		answer(tr(lang, "Меню недоступно. Пришлите видео заново."))
		return
	}
	answer("")
	p.Request.Language = lang
	if p.Cancelled {
		b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: msg.Chat.ID, MessageID: msg.ID, Text: tr(lang, "Отменено.")})
		return
	}
	if p.AwaitingTrim {
		b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: msg.Chat.ID, MessageID: msg.ID, Text: tr(lang, "Видео длиннее минуты. Использовать первые 60 секунд?"), ReplyMarkup: trimButtons(lang, parts[1])})
		return
	}
	// Leave the menu available if both processing slots are currently occupied.
	h.processVideo(ctx, b, p.Request)
}

func (h *Handler) inputLimits(lang string) string {
	return fmt.Sprintf(tr(lang, "Пришлите видео до %d MB и не длиннее %d секунд."), (h.Config.MaxInputVideoBytes+1048575)/1048576, h.Config.MaxInputVideoSeconds)
}

func (h *Handler) offerPlain(ctx context.Context, b *bot.Bot, req ProcessingRequest) {
	if !h.Config.EnableVideoToCircle || h.pending == nil {
		return
	}
	req.Mode = ""
	req.PendingToken = ""
	token, err := h.pending.Put(req, 0)
	if err != nil {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: req.ChatID, Text: tr(req.Language, "Можно сделать кружочек без субтитров. Он использует отдельный бесплатный дневной лимит."), ReplyMarkup: circleButtons(req.Language, token, true)})
}
