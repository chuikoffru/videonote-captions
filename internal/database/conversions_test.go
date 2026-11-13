package database

import (
	"sync"
	"testing"
	"time"
)

func TestPostgresPlainQuota(t *testing.T) {
	d := attributionDB(t)
	id := time.Now().UnixNano()
	d.GetOrCreateUser(id, "quota-test", "", "")
	t.Cleanup(func() {
		d.Where("user_id = ?", id).Delete(&PlainConversion{})
		d.Where("telegram_id = ?", id).Delete(&User{})
	})
	var mu sync.Mutex
	var ids []string
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := d.ReservePlainConversion(id, 5)
			if e == nil {
				mu.Lock()
				ids = append(ids, r)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(ids) != 5 {
		t.Fatalf("reserved %d", len(ids))
	}
	if err := d.FinishPlainConversion(ids[0], false); err != nil {
		t.Fatal(err)
	}
	if err := d.FinishPlainConversion(ids[0], false); err != nil {
		t.Fatal(err)
	}
	replacement, err := d.ReservePlainConversion(id, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.ReservePlainConversion(id, 5); err == nil {
		t.Fatal("double refund expanded quota")
	}
	if err = d.FinishPlainConversion(replacement, true); err != nil {
		t.Fatal(err)
	}
	if err = d.FinishPlainConversion(replacement, false); err != nil {
		t.Fatal(err)
	}
	if _, err = d.ReservePlainConversion(id, 5); err == nil {
		t.Fatal("delivered result was refunded")
	}
	var user User
	d.Where("telegram_id = ?", id).First(&user)
	if user.Credits != 0 || user.FreeVideoNotesUsed != 0 {
		t.Fatalf("caption balance changed %+v", user)
	}
	// An old pending reservation does not consume today's budget; refund stays on its original date.
	d.Model(&PlainConversion{}).Where("id = ?", ids[1]).Update("day", "2000-01-01")
	if _, err = d.ReservePlainConversion(id, 5); err != nil {
		t.Fatal(err)
	}
	d.FinishPlainConversion(ids[1], false)
	if _, err = d.ReservePlainConversion(id, 5); err == nil {
		t.Fatal("old-day refund changed today's budget")
	}
}

func TestMoscowQuotaDay(t *testing.T) {
	if got := plainQuotaDay(time.Date(2026, 11, 13, 21, 0, 0, 0, time.UTC)); got != "2026-11-14" {
		t.Fatal(got)
	}
}

func TestPostgresConversionStatistics(t *testing.T) {
	d := attributionDB(t)
	id := time.Now().UnixNano()
	d.RecordStart(id, "ru_video_to_circle", "ru")
	t.Cleanup(func() {
		d.Where("user_id = ?", id).Delete(&VideoLog{})
		d.Where("user_id = ?", id).Delete(&StartEvent{})
		d.Where("telegram_id = ?", id).Delete(&User{})
	})
	before, _, _ := d.GetStats()
	if err := d.LogVideoResult(id, 20, "plain-result", "plain_circle", "video", true); err != nil {
		t.Fatal(err)
	}
	after, _, _ := d.GetStats()
	if after.TotalPaidVideos != before.TotalPaidVideos || after.TotalFreeVideos != before.TotalFreeVideos || after.TotalPlainCircles != before.TotalPlainCircles+1 {
		t.Fatalf("caption stats changed %+v %+v", before, after)
	}
	var v VideoLog
	d.Where("user_id = ?", id).First(&v)
	if v.IsPaid || v.Operation != "plain_circle" || v.SourceKind != "video" || v.Duration != 20 {
		t.Fatalf("bad log %+v", v)
	}
	stats, err := d.GetSourceStats()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stats {
		if s.Source == "ru_video_to_circle" && (s.PaidUsers != 0 || s.CaptionActivated != 0) {
			t.Fatalf("plain counted as captions/payment %+v", s)
		}
	}
	if err = d.LogVideoResult(id, 20, "caption-result", "captioned_note", "video", true); err != nil {
		t.Fatal(err)
	}
	stats, _ = d.GetSourceStats()
	found := false
	for _, s := range stats {
		if s.Source == "ru_video_to_circle" {
			found = true
			if s.CaptionActivated < 1 {
				t.Fatalf("missing caption activation %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("source not found")
	}
}
