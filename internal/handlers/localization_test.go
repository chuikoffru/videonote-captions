package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"videonote-captions/internal/database"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"videonote-captions/internal/config"
)

func TestLanguageFallback(t *testing.T) {
	h := &Handler{}
	for _, tc := range []struct{ code, want string }{{"ru", "ru"}, {"ru-RU", "ru"}, {"en", "en"}, {"de", "en"}, {"", "ru"}} {
		if got := h.language(&models.User{LanguageCode: tc.code}); got != tc.want {
			t.Errorf("%q: %s", tc.code, got)
		}
	}
	if h.language(nil) != "ru" {
		t.Fatal("nil user fallback")
	}
}

func TestStartDeepLinks(t *testing.T) {
	for _, tc := range []struct{ text, code, want string }{{"/start en_home_hero", "ru", "Send"}, {"/start ru_home_hero", "en", "Отправь"}, {"/start", "en", "Send"}, {"/start bad!", "en", "Send"}, {"/start@demo en_home_footer", "ru", "Send"}} {
		t.Run(tc.text, func(t *testing.T) {
			var sent string
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				sent = r.FormValue("text")
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1,"chat":{"id":7,"type":"private"}}}`))}, nil
			})}
			b, err := bot.New("1:test", bot.WithSkipGetMe(), bot.WithHTTPClient(0, client))
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{Config: &config.Config{}}
			h.StartHandler(context.Background(), b, &models.Update{Message: &models.Message{Text: tc.text, From: &models.User{ID: 7, LanguageCode: tc.code}, Chat: models.Chat{ID: 7}}})
			if !strings.Contains(sent, tc.want) {
				t.Fatalf("got %q, want %q", sent, tc.want)
			}
		})
	}
}

func TestNilUpdates(t *testing.T) {
	h := &Handler{Config: &config.Config{}}
	for _, fn := range []func(context.Context, *bot.Bot, *models.Update){h.StartHandler, h.HelpHandler, h.BuyHandler, h.BuyCallbackHandler, h.CreditPreCheckoutHandler, h.CreditSuccessfulPaymentHandler, h.VideoNoteHandler, h.BroadcastHandler, h.UsageHandler, h.LanguageHandler, h.SourcesHandler} {
		fn(context.Background(), nil, nil)
		fn(context.Background(), nil, &models.Update{})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func captureBot(t *testing.T) (*bot.Bot, *[]map[string]string) {
	t.Helper()
	calls := new([]map[string]string)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		call := map[string]string{"method": r.URL.Path}
		for key := range r.MultipartForm.Value {
			call[key] = r.FormValue(key)
		}
		*calls = append(*calls, call)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1,"chat":{"id":7,"type":"private"}}}`))}, nil
	})}
	b, err := bot.New("1:test", bot.WithSkipGetMe(), bot.WithHTTPClient(0, client))
	if err != nil {
		t.Fatal(err)
	}
	return b, calls
}

func TestLocalizedBuyAndHelp(t *testing.T) {
	for _, code := range []string{"ru", "en"} {
		b, calls := captureBot(t)
		h := &Handler{Config: &config.Config{FreeVideoNotesPerMonth: 3, VideoNoteCredits: 10, CreditPackage1Amount: 50, CreditPackage1Price: 99}}
		u := &models.Update{Message: &models.Message{From: &models.User{ID: 7, LanguageCode: code}, Chat: models.Chat{ID: 7}}}
		h.HelpHandler(context.Background(), b, u)
		h.BuyHandler(context.Background(), b, u)
		if len(*calls) != 2 {
			t.Fatalf("calls=%v", *calls)
		}
		help, buy := (*calls)[0]["text"], (*calls)[1]["text"]
		if code == "en" {
			if !strings.Contains(help, "3 video notes") || !strings.Contains(help, "10 credits") || !strings.Contains(buy, "0 credits") {
				t.Fatalf("%q %q", help, buy)
			}
			if !strings.Contains((*calls)[1]["reply_markup"], "50 credits — 99 RUB") {
				t.Fatal((*calls)[1])
			}
		} else if !strings.Contains(help, "3 кружочка") || !strings.Contains(buy, "0 кредитов") {
			t.Fatalf("%q %q", help, buy)
		}
	}
}

func TestPrecheckoutWithoutDBLocalized(t *testing.T) {
	b, calls := captureBot(t)
	h := &Handler{}
	h.CreditPreCheckoutHandler(context.Background(), b, &models.Update{PreCheckoutQuery: &models.PreCheckoutQuery{ID: "q", From: &models.User{ID: 7, LanguageCode: "en"}, InvoicePayload: "credit:7:50:test"}})
	if len(*calls) != 1 || (*calls)[0]["error_message"] != "Invalid order. Please use /buy" || (*calls)[0]["ok"] != "false" {
		t.Fatalf("%v", *calls)
	}
}

func TestSourcesRequiresAdmin(t *testing.T) {
	h := &Handler{Config: &config.Config{AdminID: 9}}
	h.SourcesHandler(context.Background(), nil, &models.Update{Message: &models.Message{From: &models.User{ID: 7}}})
}

func TestPostgresLanguageAndStart(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	db, err := database.New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB.DB()
	defer sqlDB.Close()
	id := time.Now().UnixNano()
	defer db.Where("telegram_id = ?", id).Delete(&database.User{})
	defer db.Where("user_id = ?", id).Delete(&database.StartEvent{})
	b, calls := captureBot(t)
	h := &Handler{DB: db, Config: &config.Config{AdminID: id}}
	u := &models.Update{Message: &models.Message{Text: "/start en_home_hero", From: &models.User{ID: id, LanguageCode: "ru"}, Chat: models.Chat{ID: id}}}
	h.StartHandler(context.Background(), b, u)
	if lang, _ := db.GetUserLanguage(id); lang != "en" {
		t.Fatalf("hint not persisted: %s", lang)
	}
	var user database.User
	db.Where("telegram_id = ?", id).First(&user)
	if user.FirstSource != "en_home_hero" {
		t.Fatalf("source=%q", user.FirstSource)
	}
	u.Message.Text = "/language ru"
	h.LanguageHandler(context.Background(), b, u)
	// A new Handler simulates the locale lookup after a process restart.
	restarted := &Handler{DB: db, Config: h.Config}
	u.Message.From.LanguageCode = "en"
	u.Message.Text = "/start"
	restarted.StartHandler(context.Background(), b, u)
	if !strings.Contains((*calls)[len(*calls)-1]["text"], "Отправь") {
		t.Fatal("saved language lost to Telegram fallback")
	}
	if lang, _ := db.GetUserLanguage(id); lang != "ru" {
		t.Fatalf("saved locale=%s", lang)
	}
	var before int64
	db.Model(&database.StartEvent{}).Where("user_id = ?", id).Count(&before)
	u.Message.Text = "/start https://attacker.invalid/"
	restarted.StartHandler(context.Background(), b, u)
	var after int64
	db.Model(&database.StartEvent{}).Where("user_id = ?", id).Count(&after)
	if after != before {
		t.Fatal("invalid source recorded")
	}
	u.Message.Text = "/language fr"
	h.LanguageHandler(context.Background(), b, u)
	if lang, _ := db.GetUserLanguage(id); lang != "ru" {
		t.Fatal("invalid language overwrote preference")
	}
	// An English invoice retains RUB minor units and package credits.
	if err := db.SetUserLanguage(id, "en"); err != nil {
		t.Fatal(err)
	}
	h.Config.YuKassaToken = "test-provider"
	h.Config.CreditPackage1Amount = 50
	h.Config.CreditPackage1Price = 99
	h.BuyCallbackHandler(context.Background(), b, &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback", From: *u.Message.From, Data: "buy:1", Message: models.MaybeInaccessibleMessage{Message: u.Message}}})
	invoice := (*calls)[len(*calls)-1]
	if invoice["title"] != "Credit top-up" || invoice["currency"] != "RUB" || invoice["description"] != "50 credits" {
		t.Fatalf("invoice=%v", invoice)
	}
	var prices []models.LabeledPrice
	if err := json.Unmarshal([]byte(invoice["prices"]), &prices); err != nil || len(prices) != 1 || prices[0].Amount != 9900 {
		t.Fatalf("prices=%v %v", prices, err)
	}
	payload := invoice["payload"]
	defer db.Where("payload = ?", payload).Delete(&database.PaymentOrder{})
	order, err := db.GetOrder(payload)
	if err != nil || order.Amount != 50 || order.PriceRub != 99 {
		t.Fatalf("order=%+v %v", order, err)
	}
	h.CreditPreCheckoutHandler(context.Background(), b, &models.Update{PreCheckoutQuery: &models.PreCheckoutQuery{ID: "checkout", From: u.Message.From, InvoicePayload: payload, Currency: "RUB", TotalAmount: 9900}})
	if (*calls)[len(*calls)-1]["ok"] != "true" {
		t.Fatal("valid invoice rejected")
	}
	u.Message.SuccessfulPayment = &models.SuccessfulPayment{InvoicePayload: payload, Currency: "RUB", TotalAmount: 9900, TelegramPaymentChargeID: payload}
	defer db.Where("user_id = ?", id).Delete(&database.CreditTransaction{})
	h.CreditSuccessfulPaymentHandler(context.Background(), b, u)
	balance, err := db.GetUserCredits(id)
	if err != nil || balance != 50 {
		t.Fatalf("balance=%d %v", balance, err)
	}
	if !strings.Contains((*calls)[len(*calls)-1]["text"], "Balance: 50 credits") {
		t.Fatal("payment confirmation locale")
	}

	// Admin source output stays plain text, including values with underscores.
	h.SourcesHandler(context.Background(), b, u)
	last := (*calls)[len(*calls)-1]
	if last["parse_mode"] != "" || !strings.Contains(last["text"], "en_home_hero") {
		t.Fatalf("unsafe/missing source output: %v", last)
	}
}

func TestPostgresSourcesReportFitsTelegram(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	db, err := database.New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	id := time.Now().UnixNano()
	sources := []string{}
	for i := 0; i < 60; i++ {
		uid := id + int64(i)
		source := "en_" + strings.Repeat("x", 54) + fmt.Sprintf("_%06d", i)
		sources = append(sources, source)
		if err := db.RecordStart(uid, source, "en"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.Where("user_id = ?", uid).Delete(&database.StartEvent{})
			db.Where("telegram_id = ?", uid).Delete(&database.User{})
		})
	}
	b, calls := captureBot(t)
	h := &Handler{DB: db, Config: &config.Config{AdminID: id}}
	h.SourcesHandler(context.Background(), b, &models.Update{Message: &models.Message{From: &models.User{ID: id, LanguageCode: "en"}, Chat: models.Chat{ID: id}}})
	combined := ""
	for _, call := range *calls {
		text := call["text"]
		if utf8.RuneCountInString(text) > 4096 {
			t.Fatalf("Telegram rejects %d-rune message", utf8.RuneCountInString(text))
		}
		combined += text
	}
	for _, s := range sources {
		if !strings.Contains(combined, s) {
			t.Fatal("missing source", s)
		}
	}
}
