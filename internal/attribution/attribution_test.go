package attribution

import (
	"strings"
	"testing"
)

func TestCommandExact(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"/start", "start"}, {"/start@CaptionBot ru_home_hero", "start"}, {"/language en", "language"}, {"/sources", "sources"},
		{"/starter", "starter"}, {"text /start", ""}, {"/start@", ""}, {"/start@bot@other", ""}, {"/START", ""}, {" /start", ""},
	} {
		if got := Command(tc.text); got != tc.want {
			t.Errorf("Command(%q)=%q want %q", tc.text, got, tc.want)
		}
	}
}
func TestParseStart(t *testing.T) {
	for _, tc := range []struct {
		text, source, language string
		valid                  bool
	}{
		{"/start", "direct", "", true}, {"/start@CaptionBot", "direct", "", true}, {"/start   ", "direct", "", true},
		{"/start ru_home_hero", "ru_home_hero", "ru", true}, {"/start@bot en_home_footer", "en_home_footer", "en", true},
		{"/start test-1_A", "test-1_A", "", true}, {"/start " + strings.Repeat("a", 64), strings.Repeat("a", 64), "", true},
		{"/start " + strings.Repeat("a", 65), "", "", false}, {"/start https://evil.test", "", "", false}, {"/start two words", "", "", false},
		{"/starter ru_home", "", "", false}, {"/start привет", "", "", false}, {"/help", "", "", false},
	} {
		s, l, v := ParseStart(tc.text)
		if s != tc.source || l != tc.language || v != tc.valid {
			t.Errorf("ParseStart(%q)=(%q,%q,%t)", tc.text, s, l, v)
		}
	}
}
