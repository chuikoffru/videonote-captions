package handlers

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"videonote-captions/internal/audio"
	"videonote-captions/internal/database"
	"videonote-captions/internal/transcription"
	"videonote-captions/internal/video"
)

type ProcessingRequest struct {
	OwnerID, ChatID                                  int64
	SourceMessageID                                  int
	FileID, SourceKind, Mode, Language, PendingToken string
	TrimConfirmed                                    bool
}

type conversionBilling interface {
	ReservePlainConversion(int64, int) (string, error)
	FinishPlainConversion(string, bool) error
	ReserveNote(int64, int, int, bool) (bool, error)
	RefundNote(int64, bool, int, bool) error
	LogVideoResult(int64, int, string, string, string, bool) error
}

type mediaTools struct {
	Download func(context.Context, *bot.Bot, string, string, string, int64) (string, error)
	Probe    func(context.Context, string) (video.MediaInfo, error)
	Convert  func(context.Context, string, string, video.CircleOptions) error
	Extract  func(context.Context, string) (string, error)
	Caption  func(context.Context, string, *transcription.Result) (string, error)
}

func (h *Handler) processVideo(parent context.Context, b *bot.Bot, job ProcessingRequest) {
	if job.Mode != "plain" && job.Mode != "captions" {
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		if job.PendingToken != "" {
			h.pending.Reset(job.PendingToken)
		}
		b.SendMessage(parent, &bot.SendMessageParams{ChatID: job.ChatID, Text: tr(job.Language, "Бот занят обработкой. Попробуйте через минуту.")})
		return
	}
	if job.PendingToken != "" {
		defer h.pending.Complete(job.PendingToken)
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(h.Config.VideoProcessTimeoutSeconds)*time.Second)
	defer cancel()
	started := time.Now()
	stage := "start"
	outcome := "error"
	defer func() {
		log.Printf("video_processing mode=%s source=%s stage=%s outcome=%s duration_ms=%d", job.Mode, job.SourceKind, stage, outcome, time.Since(started).Milliseconds())
	}()
	status, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: job.ChatID, Text: tr(job.Language, "⬇️ Скачиваю видео..."), ReplyParameters: &models.ReplyParameters{MessageID: job.SourceMessageID}})
	if err != nil {
		return
	}
	update := func(text string) { h.updateStatus(ctx, b, job.ChatID, status.ID, tr(job.Language, text)) }
	fail := func() {
		update("Не удалось обработать видео. Лимит и кредиты не расходуются. Пришлите видео заново.")
	}
	if h.billing == nil {
		fail()
		return
	}
	if err = os.MkdirAll(h.tempRoot, 0700); err != nil {
		fail()
		return
	}
	dir, err := os.MkdirTemp(h.tempRoot, "circle-")
	if err != nil {
		fail()
		return
	}
	defer os.RemoveAll(dir)
	stage = "download"
	input, err := h.media.Download(ctx, b, job.FileID, h.Config.BotToken, dir, h.Config.MaxInputVideoBytes)
	if err != nil {
		if errors.Is(err, audio.ErrFileTooLarge) {
			h.updateStatus(ctx, b, job.ChatID, status.ID, h.inputLimits(job.Language))
		} else {
			fail()
		}
		return
	}
	stage = "probe"
	info, err := h.media.Probe(ctx, input)
	if err != nil {
		fail()
		return
	}
	if err = video.ValidateInput(info, h.Config.MaxInputVideoSeconds); err != nil {
		if errors.Is(err, video.ErrHDR) {
			update("Это HDR-видео. Экспортируйте его в SDR и пришлите снова. Лимит и кредиты не расходуются.")
		} else {
			h.updateStatus(ctx, b, job.ChatID, status.ID, h.inputLimits(job.Language))
		}
		return
	}
	if info.DurationSeconds > 60 && !job.TrimConfirmed {
		outcome = "awaiting_trim"
		if job.PendingToken != "" {
			h.pending.RequireTrim(job.PendingToken)
			b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: job.ChatID, MessageID: status.ID, Text: tr(job.Language, "Видео длиннее минуты. Использовать первые 60 секунд?"), ReplyMarkup: trimButtons(job.Language, job.PendingToken)})
		} else {
			fail()
		}
		return
	}
	if job.Mode == "captions" && !info.HasAudio {
		update("В видео нет звука или распознаваемой речи. Субтитры не созданы, лимит и кредиты не расходуются.")
		h.offerPlain(ctx, b, job)
		return
	}
	stage = "reserve"
	reservation := ""
	free := true
	admin := job.OwnerID == h.Config.AdminID
	if job.Mode == "plain" {
		reservation, err = h.billing.ReservePlainConversion(job.OwnerID, h.Config.PlainCirclesPerDay)
	} else {
		free, err = h.billing.ReserveNote(job.OwnerID, h.Config.FreeVideoNotesPerMonth, h.Config.VideoNoteCredits, admin)
	}
	if err != nil {
		if errors.Is(err, database.ErrPlainQuotaExceeded) {
			if job.PendingToken != "" {
				h.pending.Reset(job.PendingToken)
			}
			update("Дневной лимит бесплатных кружочков исчерпан. Он обновится в полночь по Москве. Субтитры доступны по текущим условиям.")
		} else if errors.Is(err, database.ErrInsufficientCredits) && h.DB != nil {
			if job.PendingToken != "" {
				h.pending.Reset(job.PendingToken)
			}
			balance, _ := h.DB.GetUserCredits(job.OwnerID)
			h.sendFreeNotesExhaustedMessage(ctx, b, job.ChatID, balance, job.Language)
			update("Недостаточно кредитов для субтитров. Используйте /buy или выберите кружочек без субтитров.")
			if job.PendingToken == "" {
				h.offerPlain(ctx, b, job)
			}
		} else {
			fail()
		}
		return
	}
	delivered := false
	defer func() {
		var e error
		if job.Mode == "plain" {
			e = h.billing.FinishPlainConversion(reservation, delivered)
		} else if !delivered {
			e = h.billing.RefundNote(job.OwnerID, free, h.Config.VideoNoteCredits, admin)
		}
		if e != nil {
			log.Printf("video_processing mode=%s reservation=%s stage=reservation_finish outcome=error", job.Mode, reservation)
		}
	}()
	stage = "convert"
	update("🎬 Готовлю кружочек...")
	output := input
	// Existing square video notes already have a compatible geometry; avoid an extra lossy pass.
	if job.SourceKind != "video_note" || job.Mode == "plain" {
		output = filepath.Join(dir, "circle.mp4")
		if err = h.media.Convert(ctx, input, output, video.CircleOptions{DurationSeconds: math.Min(info.DurationSeconds, 60)}); err != nil {
			fail()
			return
		}
	}
	var transcript *transcription.Result
	if job.Mode == "captions" {
		stage = "extract"
		update("🔊 Извлекаю звук...")
		audioPath, e := h.media.Extract(ctx, output)
		if e != nil {
			fail()
			return
		}
		stage = "transcribe"
		update("📝 Распознаю речь...")
		transcript, err = h.Transcriber.TranscribeAudio(ctx, audioPath)
		if err != nil || !hasTimedWords(transcript) {
			if err == nil || errors.Is(err, transcription.ErrNoSpeech) {
				update("В видео нет звука или распознаваемой речи. Субтитры не созданы, лимит и кредиты не расходуются.")
			} else {
				fail()
			}
			h.offerPlain(ctx, b, job)
			return
		}
		stage = "captions"
		update("🎬 Накладываю субтитры...")
		output, err = h.media.Caption(ctx, output, transcript)
		if err != nil {
			fail()
			return
		}
	}
	stage = "validate_output"
	result, err := h.media.Probe(ctx, output)
	if err != nil {
		fail()
		return
	}
	if err = video.ValidateCircleOutput(output, result); err != nil {
		fail()
		return
	}
	file, err := os.Open(output)
	if err != nil {
		fail()
		return
	}
	defer file.Close()
	stage = "send"
	update("📤 Отправляю результат...")
	duration := int(math.Ceil(math.Min(result.DurationSeconds, 60)))
	sent, err := b.SendVideoNote(ctx, &bot.SendVideoNoteParams{ChatID: job.ChatID, VideoNote: &models.InputFileUpload{Filename: "video_note.mp4", Data: file}, Duration: duration, Length: result.Width})
	if err != nil {
		fail()
		return
	}
	delivered = true
	outcome = "delivered"
	stage = "complete"
	if sent != nil && sent.VideoNote != nil {
		operation := "plain_circle"
		if job.Mode == "captions" {
			operation = "captioned_note"
		}
		if e := h.billing.LogVideoResult(job.OwnerID, duration, sent.VideoNote.FileID, operation, job.SourceKind, free); e != nil {
			log.Printf("video_processing mode=%s stage=log_result outcome=error", job.Mode)
		}
	}
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: job.ChatID, MessageID: status.ID})
	if transcript != nil && sent != nil {
		h.sendTranscript(ctx, b, job.ChatID, sent.ID, job.Language, transcript.Text)
	}
}

func hasTimedWords(result *transcription.Result) bool {
	if result == nil {
		return false
	}
	for _, w := range result.Words {
		if strings.TrimSpace(w.Word) != "" && w.Start >= 0 && w.End > w.Start && !math.IsNaN(w.Start) && !math.IsNaN(w.End) && !math.IsInf(w.Start, 0) && !math.IsInf(w.End, 0) {
			return true
		}
	}
	return false
}

func (h *Handler) sendTranscript(ctx context.Context, b *bot.Bot, chatID int64, messageID int, lang, text string) {
	// Keep large transcripts below Telegram's UTF-16 message limit, including the prefix.
	runes := []rune(text)
	for len(runes) > 0 {
		n := 1800
		if n > len(runes) {
			n = len(runes)
		}
		chunk := string(runes[:n])
		runes = runes[n:]
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: fmt.Sprint(tr(lang, "📝 Транскрипт:\n\n"), chunk), ReplyParameters: &models.ReplyParameters{MessageID: messageID}}); err != nil {
			log.Printf("video_processing stage=transcript_send outcome=error")
			return
		}
	}
}
