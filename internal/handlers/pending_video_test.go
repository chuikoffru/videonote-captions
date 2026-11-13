package handlers

import (
	"context"
	"github.com/go-telegram/bot/models"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"videonote-captions/internal/config"
)

func TestPendingChoiceOwnershipExpiryAndDuplicates(t *testing.T) {
	s := newPendingVideoStore()
	now := time.Now()
	s.now = func() time.Time { return now }
	token, err := s.Put(ProcessingRequest{OwnerID: 7, ChatID: 7, FileID: "first"}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Select(token, 8, 7, "plain"); err == nil {
		t.Fatal("foreign owner accepted")
	}
	if _, err = s.Select(token, 7, 8, "plain"); err == nil {
		t.Fatal("foreign chat accepted")
	}
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := s.Select(token, 7, 7, "plain")
			if e == nil && p.Processing {
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("started %d", count.Load())
	}
	other, _ := s.Put(ProcessingRequest{OwnerID: 7, ChatID: 7, FileID: "second"}, 20)
	p, err := s.Select(other, 7, 7, "captions")
	if err != nil || p.Request.FileID != "second" {
		t.Fatalf("wrong source %+v %v", p, err)
	}
	now = now.Add(31 * time.Minute)
	if _, err = s.Select(other, 7, 7, "plain"); err == nil {
		t.Fatal("expired menu accepted")
	}
	if _, err = newPendingVideoStore().Select(token, 7, 7, "plain"); err == nil {
		t.Fatal("menu survived restart")
	}
}

func TestPendingLongVideoRequiresSeparateConfirmation(t *testing.T) {
	s := newPendingVideoStore()
	token, _ := s.Put(ProcessingRequest{OwnerID: 7, ChatID: 7, FileID: "long"}, 90)
	if _, err := s.Select(token, 7, 7, "trim"); err == nil {
		t.Fatal("trim without mode accepted")
	}
	p, err := s.Select(token, 7, 7, "captions")
	if err != nil || p.Processing || p.Request.TrimConfirmed {
		t.Fatalf("silent trim %+v %v", p, err)
	}
	p, err = s.Select(token, 7, 7, "trim")
	if err != nil || !p.Processing || !p.Request.TrimConfirmed || p.Request.Mode != "captions" {
		t.Fatalf("confirmation %+v %v", p, err)
	}
}

func TestOrdinaryVideoMenuHasBothModes(t *testing.T) {
	b, calls := captureBot(t)
	h := NewHandler(&config.Config{EnableVideoToCircle: true, PlainCirclesPerDay: 5, FreeVideoNotesPerMonth: 3, VideoNoteCredits: 10}, nil)
	u := &models.Update{Message: &models.Message{ID: 12, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Video: &models.Video{FileID: "video", Duration: 10, FileSize: 123}}}
	h.VideoHandler(context.Background(), b, u)
	if len(*calls) != 1 || !strings.Contains((*calls)[0]["reply_markup"], "circle:") || !strings.Contains((*calls)[0]["text"], "5") {
		t.Fatalf("menu: %v", *calls)
	}
	if !strings.Contains((*calls)[0]["reply_markup"], "Сделать кружочек") || !strings.Contains((*calls)[0]["reply_markup"], "с субтитрами") {
		t.Fatal((*calls)[0])
	}
}

func TestVideoDocumentDetection(t *testing.T) {
	for _, tc := range []struct {
		doc  models.Document
		want bool
	}{{models.Document{MimeType: "video/mp4"}, true}, {models.Document{FileName: "iPhone.MOV"}, true}, {models.Document{MimeType: "application/pdf", FileName: "x.pdf"}, false}} {
		if got := AcceptsVideo(&models.Message{Document: &tc.doc}); got != tc.want {
			t.Fatalf("%+v %v", tc.doc, got)
		}
	}
	if AcceptsVideo(nil) {
		t.Fatal("nil accepted")
	}
}

func TestVideoFeatureDisabledAndInvalidCallbacks(t *testing.T) {
	b, calls := captureBot(t)
	h := NewHandler(&config.Config{}, nil)
	u := &models.Update{Message: &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Video: &models.Video{FileID: "v"}}}
	h.VideoHandler(context.Background(), b, u)
	if len(*calls) != 0 {
		t.Fatal("disabled feature offered a menu")
	}
	h.Config.EnableVideoToCircle = true
	h.CircleCallbackHandler(context.Background(), b, &models.Update{CallbackQuery: &models.CallbackQuery{ID: "q", From: models.User{ID: 7}, Data: "circle:missing:plain"}})
	if len(*calls) != 1 || !strings.HasSuffix((*calls)[0]["method"], "answerCallbackQuery") {
		t.Fatalf("invalid callback: %v", *calls)
	}
}
