package database

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestPostgresBilling(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for Postgres integration")
	}
	d, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := d.GetFullStats()
	if err != nil {
		t.Fatal(err)
	}
	id := time.Now().UnixNano()
	if _, err = d.GetOrCreateUser(id, "test", "Test", ""); err != nil {
		t.Fatal(err)
	}
	defer d.Where("telegram_id = ?", id).Delete(&User{})
	defer d.Where("user_id = ?", id).Delete(&PaymentOrder{})
	defer d.Where("user_id = ?", id).Delete(&CreditTransaction{})
	payload := "test-order-" + time.Now().Format("150405.000000000")
	if err = d.CreateOrder(PaymentOrder{Payload: payload, UserID: id, Amount: 100, PriceRub: 99}); err != nil {
		t.Fatal(err)
	}
	if err = d.CompleteOrder(payload, id, 1, "RUB", "bad"); err == nil {
		t.Fatal("accepted wrong amount")
	}
	// A second connection represents a bot restart; pending invoices must survive it.
	restarted, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- restarted.CompleteOrder(payload, id, 9900, "RUB", payload) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	balance, _ := d.GetUserCredits(id)
	if balance != 100 {
		t.Fatalf("duplicate payment: balance %d", balance)
	}
	// Concurrent requests may only consume the quota once, then the balance.
	var mu sync.Mutex
	freeCount, paidCount := 0, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			free, e := d.ReserveNote(id, 3, 10, false)
			if e == nil {
				mu.Lock()
				if free {
					freeCount++
				} else {
					paidCount++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if freeCount != 3 || paidCount != 10 {
		t.Fatalf("free=%d paid=%d", freeCount, paidCount)
	}
	if err = d.RefundNote(id, false, 10, false); err != nil {
		t.Fatal(err)
	}
	balance, _ = d.GetUserCredits(id)
	if balance != 10 {
		t.Fatalf("refund balance %d", balance)
	}
	if err = d.RefundNote(id, true, 10, false); err != nil {
		t.Fatal(err)
	}
	stats, err := d.GetFullStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalSpent-baseline.TotalSpent != 90 {
		t.Fatalf("net spent after refund = %d, want 90", stats.TotalSpent-baseline.TotalSpent)
	}
	left, err := d.GetFreeVideoNotesRemaining(id, 3)
	if err != nil || left != 1 {
		t.Fatalf("quota refund: %d %v", left, err)
	}
}
