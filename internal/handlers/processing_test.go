package handlers

import (
	"context"
	"errors"
	"github.com/go-telegram/bot"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videonote-captions/internal/config"
	"videonote-captions/internal/database"
	"videonote-captions/internal/transcription"
	"videonote-captions/internal/video"
)

type fakeVideoBilling struct {
	plain, caption, refunded, finished, logged int
	delivered                                  bool
	reserveErr                                 error
}

func (f *fakeVideoBilling) ReservePlainConversion(int64, int) (string, error) {
	f.plain++
	return "reservation", f.reserveErr
}
func (f *fakeVideoBilling) FinishPlainConversion(_ string, ok bool) error {
	f.finished++
	f.delivered = ok
	return nil
}
func (f *fakeVideoBilling) ReserveNote(int64, int, int, bool) (bool, error) {
	f.caption++
	return true, f.reserveErr
}
func (f *fakeVideoBilling) RefundNote(int64, bool, int, bool) error { f.refunded++; return nil }
func (f *fakeVideoBilling) LogVideoResult(int64, int, string, string, string, bool) error {
	f.logged++
	return nil
}

type fakeTranscriber struct {
	calls         int
	fail, noWords bool
}

func (f *fakeTranscriber) TranscribeAudio(context.Context, string) (*transcription.Result, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("ASR failed")
	}
	if f.noWords {
		return &transcription.Result{}, nil
	}
	return &transcription.Result{Text: "hello", Words: []transcription.Word{{Word: "hello", Start: 0, End: 0.5}}}, nil
}

func processingHarness(t *testing.T, stage string) (*Handler, *fakeVideoBilling, *fakeTranscriber, *[]string, *bot.Bot) {
	t.Helper()
	billing := &fakeVideoBilling{}
	tr := &fakeTranscriber{fail: stage == "asr", noWords: stage == "no-words"}
	h := NewHandler(&config.Config{EnableVideoToCircle: true, PlainCirclesPerDay: 5, FreeVideoNotesPerMonth: 3, VideoNoteCredits: 10, MaxInputVideoBytes: 20971520, MaxInputVideoSeconds: 600, VideoProcessTimeoutSeconds: 180}, nil)
	h.billing = billing
	h.Transcriber = tr
	h.tempRoot = t.TempDir()
	h.media = mediaTools{
		Download: func(_ context.Context, _ *bot.Bot, _, _, dir string, _ int64) (string, error) {
			p := filepath.Join(dir, "input.mp4")
			os.WriteFile(p, []byte("input"), 0600)
			if stage == "download" {
				return "", errors.New(stage)
			}
			return p, nil
		},
		Probe: func(_ context.Context, p string) (video.MediaInfo, error) {
			if stage == "probe" {
				return video.MediaInfo{}, errors.New(stage)
			}
			duration := 1.0
			if stage == "long" && strings.HasSuffix(p, "input.mp4") {
				duration = 80
			}
			return video.MediaInfo{Width: 384, Height: 384, DurationSeconds: duration, HasAudio: stage != "silent", VideoCodec: "h264", AudioCodec: "aac", PixelFormat: "yuv420p", SampleAspectRatio: "1:1"}, nil
		},
		Convert: func(_ context.Context, _, out string, _ video.CircleOptions) error {
			if stage == "convert" {
				return errors.New(stage)
			}
			return os.WriteFile(out, []byte("output"), 0600)
		},
		Extract: func(_ context.Context, p string) (string, error) {
			if stage == "extract" {
				return "", errors.New(stage)
			}
			return p, nil
		},
		Caption: func(_ context.Context, p string, _ *transcription.Result) (string, error) {
			if stage == "caption" {
				return "", errors.New(stage)
			}
			return p, nil
		},
	}
	methods := new([]string)
	testBot, _ := bot.New("1:test", bot.WithSkipGetMe(), bot.WithHTTPClient(0, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Body != nil {
			io.Copy(io.Discard, r.Body)
		}
		*methods = append(*methods, r.URL.Path)
		if stage == "send" && strings.HasSuffix(r.URL.Path, "/sendVideoNote") {
			return nil, errors.New("send failed")
		}
		body := `{"ok":true,"result":{"message_id":1,"chat":{"id":7,"type":"private"},"video_note":{"file_id":"result","file_unique_id":"u","length":384,"duration":1}}}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	return h, billing, tr, methods, testBot
}

func TestProcessingModesAndFailures(t *testing.T) {
	for _, mode := range []string{"plain", "captions"} {
		for _, stage := range []string{"ok", "download", "probe", "convert", "extract", "asr", "caption", "send", "silent", "no-words"} {
			if mode == "plain" && (stage == "extract" || stage == "asr" || stage == "caption" || stage == "no-words") {
				continue
			}
			t.Run(mode+"/"+stage, func(t *testing.T) {
				h, billing, tr, methods, testBot := processingHarness(t, stage)
				h.processVideo(context.Background(), testBot, ProcessingRequest{OwnerID: 7, ChatID: 7, SourceMessageID: 12, FileID: "input", SourceKind: "video", Mode: mode})
				success := stage == "ok" || (mode == "plain" && stage == "silent")
				sent := 0
				for _, m := range *methods {
					if strings.HasSuffix(m, "/sendVideoNote") {
						sent++
					}
				}
				if success && (sent != 1 || billing.logged != 1) {
					t.Fatalf("not delivered: %v %+v", *methods, billing)
				}
				if !success && billing.logged != 0 {
					t.Fatal("failed result logged")
				}
				if mode == "plain" {
					if tr.calls != 0 || billing.caption != 0 || billing.refunded != 0 {
						t.Fatalf("plain touched captions: %+v %+v", tr, billing)
					}
					if billing.plain > 0 && (billing.finished != 1 || billing.delivered != success) {
						t.Fatalf("plain completion %+v", billing)
					}
				} else {
					if billing.plain != 0 {
						t.Fatal("captions consumed daily quota")
					}
					if billing.caption > 0 && !success && billing.refunded != 1 {
						t.Fatalf("missing refund %+v", billing)
					}
				}
				entries, _ := os.ReadDir(h.tempRoot)
				if len(entries) != 0 || len(h.slots) != 0 {
					t.Fatal("temporary files or slot leaked")
				}
			})
		}
	}
}

func TestProcessingLongVideoDoesNotReserveOrTrimSilently(t *testing.T) {
	h, billing, tr, methods, testBot := processingHarness(t, "long")
	token, _ := h.pending.Put(ProcessingRequest{OwnerID: 7, ChatID: 7, FileID: "input", SourceKind: "video"}, 0)
	p, _ := h.pending.Select(token, 7, 7, "plain")
	p.Request.PendingToken = token
	h.processVideo(context.Background(), testBot, p.Request)
	if billing.plain != 0 || billing.caption != 0 || tr.calls != 0 {
		t.Fatalf("reserved before trim %+v", billing)
	}
	for _, m := range *methods {
		if strings.HasSuffix(m, "/sendVideoNote") {
			t.Fatal("silently trimmed")
		}
	}
	p, err := h.pending.Select(token, 7, 7, "trim")
	if err != nil || !p.Request.TrimConfirmed {
		t.Fatalf("no confirmation available: %+v %v", p, err)
	}
}

func TestBusyVideoKeepsMenuAndDoesNotReserve(t *testing.T) {
	h, billing, tr, _, testBot := processingHarness(t, "ok")
	h.slots <- struct{}{}
	h.slots <- struct{}{}
	token, _ := h.pending.Put(ProcessingRequest{OwnerID: 7, ChatID: 7, FileID: "input", SourceKind: "video"}, 1)
	p, _ := h.pending.Select(token, 7, 7, "plain")
	h.processVideo(context.Background(), testBot, p.Request)
	if billing.plain != 0 || billing.caption != 0 || tr.calls != 0 {
		t.Fatal("busy job reserved resources")
	}
	if _, err := h.pending.Select(token, 7, 7, "plain"); err != nil {
		t.Fatal("busy menu was consumed", err)
	}
	<-h.slots
	<-h.slots
}

func TestDailyLimitStillAllowsCaptionSelection(t *testing.T) {
	h, billing, _, _, testBot := processingHarness(t, "ok")
	billing.reserveErr = database.ErrPlainQuotaExceeded
	token, _ := h.pending.Put(ProcessingRequest{OwnerID: 7, ChatID: 7, FileID: "input", SourceKind: "video"}, 1)
	p, _ := h.pending.Select(token, 7, 7, "plain")
	h.processVideo(context.Background(), testBot, p.Request)
	if _, err := h.pending.Select(token, 7, 7, "captions"); err != nil {
		t.Fatal("daily quota prevented choosing captions", err)
	}
}
