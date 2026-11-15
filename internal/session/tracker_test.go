package session_test

import (
	"testing"
	"time"

	"token-ray/internal/session"
)

func TestWindowUsage_EmptyTracker(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)
	tokens, cost := tr.WindowUsage()
	if tokens != 0 || cost != 0 {
		t.Errorf("empty tracker: expected 0/0, got %d/%.2f", tokens, cost)
	}
}

func TestWindowUsage_WithinWindow(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)

	now := time.Now()
	tr.Record(1000, 1.50, now.Add(-1*time.Hour))
	tr.Record(2000, 3.00, now.Add(-30*time.Minute))

	tokens, cost := tr.WindowUsage()
	if tokens != 3000 {
		t.Errorf("expected 3000 tokens, got %d", tokens)
	}
	if cost < 4.49 || cost > 4.51 {
		t.Errorf("expected ~4.50 cost, got %.2f", cost)
	}
}

func TestWindowUsage_ExpiredEvicted(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)

	now := time.Now()
	// This one is 6h ago — outside the 5h window
	tr.Record(5000, 10.00, now.Add(-6*time.Hour))
	// This one is 1h ago — inside
	tr.Record(1000, 2.00, now.Add(-1*time.Hour))

	tokens, cost := tr.WindowUsage()
	if tokens != 1000 {
		t.Errorf("expected 1000 tokens (old evicted), got %d", tokens)
	}
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("expected ~2.00 cost, got %.2f", cost)
	}
}

func TestBurnRate(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)

	now := time.Now()
	// 6000 tokens over the last 60 minutes = 100 tok/min
	tr.Record(3000, 1.00, now.Add(-60*time.Minute))
	tr.Record(3000, 1.00, now.Add(-1*time.Minute))

	rate := tr.BurnRate()
	// Rate should be around 100 tok/min (6000 tokens / 59 minutes)
	if rate < 80 || rate > 120 {
		t.Errorf("expected burn rate ~100 tok/min, got %.1f", rate)
	}
}

func TestBurnRate_SingleEntry(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)
	tr.Record(1000, 1.00, time.Now())

	rate := tr.BurnRate()
	if rate != 0 {
		t.Errorf("single entry should have 0 burn rate, got %.1f", rate)
	}
}

func TestTimeToExhaustion(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour) // pro = 19000 tokens

	now := time.Now()
	// Already used 9500 tokens, burn rate = ~100 tok/min
	tr.Record(4750, 1.00, now.Add(-60*time.Minute))
	tr.Record(4750, 1.00, now.Add(-1*time.Minute))

	ttl := tr.TimeToExhaustion()
	// remaining = 19000 - 9500 = 9500, rate ~161 tok/min → ~59 min
	// Allow wide tolerance since this is a rough estimate
	if ttl <= 0 {
		t.Errorf("expected positive time to exhaustion, got %v", ttl)
	}
	if ttl > 5*time.Hour {
		t.Errorf("time to exhaustion seems too high: %v", ttl)
	}
}

func TestTimeToExhaustion_AlreadyExhausted(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour) // pro = 19000 tokens

	now := time.Now()
	// Already used 20000 tokens — over the limit
	tr.Record(20000, 50.00, now.Add(-1*time.Minute))

	ttl := tr.TimeToExhaustion()
	if ttl != 0 {
		t.Errorf("exhausted tracker should return 0, got %v", ttl)
	}
}

func TestWindowReset(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)

	now := time.Now()
	tr.Record(1000, 1.00, now.Add(-2*time.Hour))

	reset := tr.WindowReset()
	// Should be ~3 hours from now (first entry was 2h ago, window is 5h)
	remaining := time.Until(reset)
	if remaining < 2*time.Hour || remaining > 4*time.Hour {
		t.Errorf("expected reset in ~3h, got %v", remaining)
	}
}

func TestWindowReset_NoData(t *testing.T) {
	tr := session.NewTracker("pro", 5*time.Hour)
	reset := tr.WindowReset()
	if !reset.IsZero() {
		t.Errorf("no data should return zero time, got %v", reset)
	}
}
