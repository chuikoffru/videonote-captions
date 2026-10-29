package database

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func attributionDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for Postgres integration")
	}
	d, e := New(dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { sqlDB, _ := d.DB.DB(); sqlDB.Close() })
	return d
}
func TestPostgresStartPersistence(t *testing.T) {
	d := attributionDB(t)
	id := time.Now().UnixNano()
	t.Cleanup(func() {
		d.Where("user_id = ?", id).Delete(&StartEvent{})
		d.Where("telegram_id = ?", id).Delete(&User{})
	})
	if _, e := d.GetOrCreateUser(id, "test", "", " "); e != nil {
		t.Fatal(e)
	}
	d.Model(&User{}).Where("telegram_id = ?", id).Updates(map[string]interface{}{"credits": 123, "free_video_notes_used": 2})
	if e := d.RecordStart(id, "direct", "en"); e != nil {
		t.Fatal(e)
	}
	var before User
	d.Where("telegram_id = ?", id).First(&before)
	restarted := attributionDB(t)
	if e := restarted.RecordStart(id, "ru_home_hero", "ru"); e != nil {
		t.Fatal(e)
	}
	if _, e := restarted.GetOrCreateUser(id, "renamed", "", " "); e != nil {
		t.Fatal(e)
	}
	var got User
	d.Where("telegram_id = ?", id).First(&got)
	if got.FirstSource != "direct" || got.LastSource != "ru_home_hero" || got.Language != "en" || got.FirstStartedAt == nil || !got.FirstStartedAt.Equal(*before.FirstStartedAt) || got.Credits != 123 || got.FreeVideoNotesUsed != 2 {
		t.Fatalf("start reset state: %+v", got)
	}
	if e := d.SetUserLanguage(id, "ru"); e != nil {
		t.Fatal(e)
	}
	if e := d.RecordStart(id, "direct", "en"); e != nil {
		t.Fatal(e)
	}
	lang, e := restarted.GetUserLanguage(id)
	if e != nil || lang != "ru" {
		t.Fatalf("language %s %v", lang, e)
	}
	if e = d.SetUserLanguage(id, "fr"); e == nil {
		t.Fatal("accepted unsupported language")
	}
	if e = d.RecordStart(id, "bad source", "en"); e == nil {
		t.Fatal("accepted invalid source")
	}
	var events int64
	d.Model(&StartEvent{}).Where("user_id = ?", id).Count(&events)
	if events != 3 {
		t.Fatalf("events=%d", events)
	}
	d.Where("telegram_id = ?", id).First(&got)
	if got.LastSource != "direct" {
		t.Fatal("direct visit not last source")
	}
}
func TestPostgresConcurrentFirstTouch(t *testing.T) {
	d := attributionDB(t)
	id := time.Now().UnixNano()
	t.Cleanup(func() {
		d.Where("user_id = ?", id).Delete(&StartEvent{})
		d.Where("telegram_id = ?", id).Delete(&User{})
	})
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs <- d.RecordStart(id, fmt.Sprintf("en_concurrent_%d", i), "en") }(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var u User
	d.Where("telegram_id = ?", id).First(&u)
	var first StartEvent
	d.Where("user_id = ?", id).Order("id").First(&first)
	var count int64
	d.Model(&StartEvent{}).Where("user_id = ?", id).Count(&count)
	if count != 20 || u.FirstSource != first.Source || u.Language != "en" {
		t.Fatalf("concurrent start lost: %+v first=%+v count=%d", u, first, count)
	}
}
func TestPostgresSourceStatsDistinctUsers(t *testing.T) {
	d := attributionDB(t)
	id := time.Now().UnixNano()
	source := fmt.Sprintf("test_%d", id)
	for i := int64(0); i < 3; i++ {
		uid := id + i
		t.Cleanup(func() {
			d.Where("user_id = ?", uid).Delete(&VideoLog{})
			d.Where("user_id = ?", uid).Delete(&CreditTransaction{})
			d.Where("user_id = ?", uid).Delete(&StartEvent{})
			d.Where("telegram_id = ?", uid).Delete(&User{})
		})
		if e := d.RecordStart(uid, source, "ru"); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 3; i++ {
		if e := d.Create(&VideoLog{UserID: id, VideoFileID: "success"}).Error; e != nil {
			t.Fatal(e)
		}
		if e := d.Create(&CreditTransaction{UserID: id, Type: "purchase", Amount: 100}).Error; e != nil {
			t.Fatal(e)
		}
	}
	d.Create(&VideoLog{UserID: id + 1, VideoFileID: "success"})
	d.Create(&CreditTransaction{UserID: id + 1, Type: "refund", Amount: 100})
	d.Create(&VideoLog{UserID: id + 2}) // no delivered file: not activated
	if e := d.RecordStart(id, "direct", "en"); e != nil {
		t.Fatal(e)
	}
	stats, e := d.GetSourceStats()
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range stats {
		if s.Source == source {
			if s.Users != 3 || s.Activated != 2 || s.PaidUsers != 1 {
				t.Fatalf("stats=%+v", s)
			}
			return
		}
	}
	t.Fatal("missing source group")
}

func TestPostgresSourceStatsExcludeHistoricalConversions(t *testing.T) {
	d := attributionDB(t)
	id := time.Now().UnixNano()
	source := fmt.Sprintf("en_returning_%d", id)
	t.Cleanup(func() {
		d.Where("user_id = ?", id).Delete(&VideoLog{})
		d.Where("user_id = ?", id).Delete(&CreditTransaction{})
		d.Where("user_id = ?", id).Delete(&StartEvent{})
		d.Where("telegram_id = ?", id).Delete(&User{})
	})
	if _, err := d.GetOrCreateUser(id, "", "", ""); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := d.Create(&VideoLog{UserID: id, VideoFileID: "past", CreatedAt: old}).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Create(&CreditTransaction{UserID: id, Type: "purchase", Amount: 100, CreatedAt: old}).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.RecordStart(id, source, "en"); err != nil {
		t.Fatal(err)
	}
	check := func(activated, paid int64) {
		t.Helper()
		stats, err := d.GetSourceStats()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range stats {
			if s.Source == source {
				if s.Activated != activated || s.PaidUsers != paid {
					t.Fatalf("conversion leakage: %+v", s)
				}
				return
			}
		}
		t.Fatal("missing cohort")
	}
	check(0, 0)
	d.Create(&VideoLog{UserID: id, VideoFileID: "new"})
	d.Create(&CreditTransaction{UserID: id, Type: "purchase", Amount: 100})
	check(1, 1)
}
