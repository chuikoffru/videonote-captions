package handlers

import (
	"context"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// VideoNoteHandler preserves the existing direct video note to captions workflow.
func (h *Handler) VideoNoteHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.VideoNote == nil || update.Message.From == nil {
		return
	}
	msg := update.Message
	if h.DB == nil {
		return
	}
	if _, err := h.DB.GetOrCreateUser(msg.From.ID, msg.From.Username, msg.From.FirstName, msg.From.LastName); err != nil {
		return
	}
	h.processVideo(ctx, b, ProcessingRequest{OwnerID: msg.From.ID, ChatID: msg.Chat.ID, SourceMessageID: msg.ID, FileID: msg.VideoNote.FileID, SourceKind: "video_note", Mode: "captions", Language: h.language(msg.From)})
}
