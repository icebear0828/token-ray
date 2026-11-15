package session

import (
	"sync"
	"time"

	"token-ray/internal/model"
)

type entry struct {
	tokens int
	cost   float64
	ts     time.Time
}

// Tracker monitors token usage within a rolling time window for subscription plans.
type Tracker struct {
	windowSize time.Duration
	plan       string
	limit      model.PlanLimit
	mu         sync.Mutex
	entries    []entry
}

// NewTracker creates a session tracker for the given plan and window size.
func NewTracker(plan string, windowSize time.Duration) *Tracker {
	limit := model.PlanLimits[plan]
	return &Tracker{
		windowSize: windowSize,
		plan:       plan,
		limit:      limit,
	}
}

// Record adds a usage event to the tracker.
func (t *Tracker) Record(tokens int, cost float64, ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, entry{tokens: tokens, cost: cost, ts: ts})
	t.evict()
}

// evict removes entries older than the window. Must be called with lock held.
func (t *Tracker) evict() {
	cutoff := time.Now().Add(-t.windowSize)
	i := 0
	for i < len(t.entries) && t.entries[i].ts.Before(cutoff) {
		i++
	}
	if i > 0 {
		t.entries = t.entries[i:]
	}
}

// WindowUsage returns total tokens and cost within the current window.
func (t *Tracker) WindowUsage() (int, float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.evict()

	var tokens int
	var cost float64
	for _, e := range t.entries {
		tokens += e.tokens
		cost += e.cost
	}
	return tokens, cost
}

// BurnRate returns the consumption rate in tokens per minute based on entries in the window.
// Returns 0 if fewer than 2 entries exist.
func (t *Tracker) BurnRate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.evict()

	if len(t.entries) < 2 {
		return 0
	}

	first := t.entries[0].ts
	last := t.entries[len(t.entries)-1].ts
	elapsed := last.Sub(first).Minutes()
	if elapsed <= 0 {
		return 0
	}

	var total int
	for _, e := range t.entries {
		total += e.tokens
	}
	return float64(total) / elapsed
}

// TimeToExhaustion predicts how long until the plan's token limit is exhausted
// based on the current burn rate. Returns 0 if already exhausted or rate is 0.
func (t *Tracker) TimeToExhaustion() time.Duration {
	tokens, _ := t.WindowUsage()
	remaining := t.limit.TokenLimit - tokens
	if remaining <= 0 {
		return 0
	}

	rate := t.BurnRate()
	if rate <= 0 {
		return 0
	}

	minutes := float64(remaining) / rate
	return time.Duration(minutes * float64(time.Minute))
}

// WindowReset returns the time when the oldest entry in the window will expire.
// Returns zero time if no entries exist.
func (t *Tracker) WindowReset() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.evict()

	if len(t.entries) == 0 {
		return time.Time{}
	}
	return t.entries[0].ts.Add(t.windowSize)
}
