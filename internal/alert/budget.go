package alert

import (
	"log"
	"time"

	"token-ray/internal/db"
)

// AlertLevel indicates the severity of a budget status.
type AlertLevel int

const (
	LevelGreen  AlertLevel = iota // < 50%
	LevelYellow                   // 50-80%
	LevelRed                      // > 80%
)

// BudgetStatus is the result of a budget check.
type BudgetStatus struct {
	Spent     float64
	Remaining float64
	Percent   float64
	Exceeded  bool
	Level     AlertLevel
}

// BudgetAlert checks daily spending against a configured budget.
type BudgetAlert struct {
	db         *db.DB
	dailyLimit float64
	now        func() time.Time
}

// NewBudgetAlert creates a budget alert checker.
// A dailyLimit of 0 disables the alert (always green).
func NewBudgetAlert(database *db.DB, dailyLimit float64) *BudgetAlert {
	return NewBudgetAlertWithClock(database, dailyLimit, time.Now)
}

// NewBudgetAlertWithClock creates a budget alert checker with an injected clock.
func NewBudgetAlertWithClock(database *db.DB, dailyLimit float64, now func() time.Time) *BudgetAlert {
	if now == nil {
		now = time.Now
	}
	return &BudgetAlert{db: database, dailyLimit: dailyLimit, now: now}
}

// Check queries today's total cost and returns the budget status.
func (b *BudgetAlert) Check() BudgetStatus {
	if b.dailyLimit <= 0 {
		return BudgetStatus{Level: LevelGreen}
	}

	now := b.now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	spent, _, _, _, _, err := b.db.QueryPeriodStatsSince(startOfDay, "all", "")
	if err != nil {
		log.Printf("Budget check failed to query DB: %v", err)
	}

	remaining := b.dailyLimit - spent
	if remaining < 0 {
		remaining = 0
	}

	pct := (spent / b.dailyLimit) * 100
	exceeded := spent >= b.dailyLimit

	level := LevelGreen
	if pct >= 80 {
		level = LevelRed
	} else if pct >= 50 {
		level = LevelYellow
	}

	return BudgetStatus{
		Spent:     spent,
		Remaining: remaining,
		Percent:   pct,
		Exceeded:  exceeded,
		Level:     level,
	}
}
