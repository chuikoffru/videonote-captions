package landing

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{BaseURL: "https://captions.example", BotUsername: "CaptionTestBot", Indexable: true, FreeNotes: 7, NoteCredits: 12, Packages: [3]Package{{120, 109}, {360, 289}, {1200, 849}}}
}
func response(c Config, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	NewHandler(c).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}
func TestRoutesAndMetadata(t *testing.T) {
	c := testConfig()
	for _, p := range []string{"/", "/ru/", "/en/", "/ru/privacy/", "/en/privacy/", "/ru/terms/", "/en/terms/", "/healthz", "/robots.txt", "/sitemap.xml", "/assets/site.css", "/assets/demo.svg"} {
		if w := response(c, p); w.Code != 200 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	for _, p := range []string{"/ru", "/en"} {
		w := response(c, p)
		if w.Code != 301 || w.Header().Get("Location") != p+"/" {
			t.Errorf("redirect %s", p)
		}
	}
	if w := response(c, "/missing"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	for _, lang := range []string{"ru", "en"} {
		b := response(c, "/"+lang+"/").Body.String()
		for _, s := range []string{`<html lang="` + lang + `"`, `rel="canonical" href="https://captions.example/` + lang + `/"`, `hreflang="ru"`, `hreflang="en"`, `hreflang="x-default"`, `property="og:title"`, `name="description"`, `start=` + lang + `_home_hero`, `start=` + lang + `_home_footer`, "109", "289", "849", "120", "360", "1200"} {
			if !strings.Contains(b, s) {
				t.Errorf("%s missing %s", lang, s)
			}
		}
		if strings.Count(b, "<h1") != 1 {
			t.Error("one H1 required")
		}
	}
	ru := response(c, "/ru/").Body.String()
	en := response(c, "/en/").Body.String()
	if !strings.Contains(ru, "7") || !strings.Contains(en, "RUB") || !strings.Contains(en, "provider") || !strings.Contains(en, "Illustrative demo") {
		t.Fatal("localized product terms missing")
	}
}
func TestIndexingIsFailClosed(t *testing.T) {
	for _, c := range []Config{{}, {BaseURL: "javascript:alert(1)", Indexable: true}, {BaseURL: "https://preview.traefik.me", Indexable: false}} {
		for _, p := range []string{"/ru/", "/en/privacy/"} {
			w := response(c, p)
			if w.Header().Get("X-Robots-Tag") != "noindex, nofollow" || !strings.Contains(w.Body.String(), `content="noindex, nofollow"`) {
				t.Error("preview can index")
			}
		}
		if !strings.Contains(response(c, "/robots.txt").Body.String(), "Disallow: /") {
			t.Error("preview robots")
		}
		if strings.Contains(response(c, "/sitemap.xml").Body.String(), "<loc>") {
			t.Error("preview sitemap should be empty")
		}
	}
	if !strings.Contains(response(testConfig(), "/robots.txt").Body.String(), "Sitemap: https://captions.example/sitemap.xml") {
		t.Fatal("indexable sitemap missing")
	}
}
func TestUnsafeConfigurationCannotBecomeLinks(t *testing.T) {
	c := testConfig()
	c.BotUsername = `evil/../x?start=other"><script>alert(1)</script>`
	c.BaseURL = `https://example.com/"><script>alert(2)</script>`
	b := response(c, "/en/").Body.String()
	if strings.Contains(b, "https://t.me/") || strings.Contains(b, "<script>") || strings.Contains(b, "javascript:") {
		t.Fatal("unsafe config rendered")
	}
	if !strings.Contains(b, "temporarily unavailable") {
		t.Fatal("missing useful disabled CTA explanation")
	}
	c = testConfig()
	c.BotUsername = "@CaptionTestBot"
	if !strings.Contains(response(c, "/en/").Body.String(), "https://t.me/CaptionTestBot?start=en_home_hero") {
		t.Fatal("valid username with @")
	}
}
func TestLegalPagesAndSecurityHeaders(t *testing.T) {
	for _, p := range []string{"/en/privacy/", "/en/terms/"} {
		w := response(testConfig(), p)
		if !strings.Contains(w.Body.String(), `hreflang="ru" href="https://captions.example/ru/`) {
			t.Error("legal equivalents missing")
		}
		if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Error("security headers missing")
		}
	}
	w := httptest.NewRecorder()
	NewHandler(testConfig()).ServeHTTP(w, httptest.NewRequest("POST", "/en/", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}

func TestRussianFreeAllowanceDeclension(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want string
	}{{0, "кружочков"}, {1, "кружочек"}, {2, "кружочка"}, {3, "кружочка"}, {4, "кружочка"}, {5, "кружочков"}, {11, "кружочков"}, {12, "кружочков"}, {21, "кружочек"}, {23, "кружочка"}, {111, "кружочков"}} {
		c := testConfig()
		c.FreeNotes = tt.n
		b := response(c, "/ru/").Body.String()
		if strings.Count(b, tt.want+" бесплатно в месяц") != 2 {
			t.Errorf("%d: expected hero and pricing to use %q", tt.n, tt.want)
		}
	}
}

func TestHTTPPreviewKeepsLanguageMetadataWithoutIndexing(t *testing.T) {
	c := testConfig()
	c.BaseURL = "http://captions-212-193-26-194.sslip.io"
	c.Indexable = true
	w := response(c, "/ru/")
	b := w.Body.String()
	if w.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatal("HTTP preview must not index")
	}
	for _, want := range []string{`rel="canonical" href="http://captions-212-193-26-194.sslip.io/ru/"`, `hreflang="en" href="http://captions-212-193-26-194.sslip.io/en/"`} {
		if !strings.Contains(b, want) {
			t.Fatalf("missing preview metadata: %s", want)
		}
	}
}
