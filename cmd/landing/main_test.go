package main

import "testing"

func TestLoadConfigDefaultsAndPublicOverrides(t *testing.T) {
	for _, k := range []string{"PORT", "BASE_URL", "BOT_USERNAME", "SITE_INDEXABLE", "FREE_VIDEO_NOTES_PER_MONTH", "VIDEO_NOTE_CREDITS", "CREDIT_PACKAGE_1_AMOUNT", "CREDIT_PACKAGE_1_PRICE", "CREDIT_PACKAGE_2_AMOUNT", "CREDIT_PACKAGE_2_PRICE", "CREDIT_PACKAGE_3_AMOUNT", "CREDIT_PACKAGE_3_PRICE"} {
		t.Setenv(k, "")
	}
	c, port := loadConfig()
	if port != "8080" || c.Indexable || c.BotUsername != "" || c.FreeNotes != 3 || c.NoteCredits != 10 || c.Packages[0].Credits != 100 || c.Packages[2].PriceRub != 749 {
		t.Fatalf("bad defaults: %+v %s", c, port)
	}
	t.Setenv("PORT", "9090")
	t.Setenv("BASE_URL", "https://example.org")
	t.Setenv("BOT_USERNAME", "ExampleBot")
	t.Setenv("SITE_INDEXABLE", "true")
	t.Setenv("FREE_VIDEO_NOTES_PER_MONTH", "0")
	t.Setenv("VIDEO_NOTE_CREDITS", "20")
	t.Setenv("CREDIT_PACKAGE_1_PRICE", "109")
	c, port = loadConfig()
	if port != "9090" || !c.Indexable || c.BaseURL != "https://example.org" || c.BotUsername != "ExampleBot" || c.FreeNotes != 0 || c.NoteCredits != 20 || c.Packages[0].PriceRub != 109 {
		t.Fatalf("overrides: %+v", c)
	}
}
func TestInvalidConfigValuesKeepSafeDefaults(t *testing.T) {
	t.Setenv("PORT", "../80")
	t.Setenv("SITE_INDEXABLE", "sure")
	t.Setenv("VIDEO_NOTE_CREDITS", "-1")
	t.Setenv("FREE_VIDEO_NOTES_PER_MONTH", "-2")
	t.Setenv("CREDIT_PACKAGE_2_PRICE", "0")
	c, p := loadConfig()
	if p != "8080" || c.Indexable || c.NoteCredits != 10 || c.FreeNotes != 3 || c.Packages[1].PriceRub != 249 {
		t.Fatalf("invalid: %+v %s", c, p)
	}
}
