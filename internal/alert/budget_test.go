package alert_test

import (
	"testing"
	"time"

	"token-ray/internal/alert"
	"token-ray/internal/model"
	"token-ray/internal/testutil"
)

var pacificTime = time.FixedZone("PDT", -7*60*60)

func TestBudgetCheck_UnderBudget(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, pacificTime)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		5.00, now.Add(-1*time.Hour), "/a.jsonl", "local",
	)

	ba := alert.NewBudgetAlertWithClock(database, 50.00, func() time.Time { return now }) // $50 daily budget
	status := ba.Check()

	if status.Spent < 4.99 || status.Spent > 5.01 {
		t.Errorf("expected spent ~5.00, got %f", status.Spent)
	}
	if status.Remaining < 44.99 {
		t.Errorf("expected remaining ~45.00, got %f", status.Remaining)
	}
	if status.Exceeded {
		t.Error("should not be exceeded")
	}
	if status.Level != alert.LevelGreen {
		t.Errorf("expected green, got %v", status.Level)
	}
}

func TestBudgetCheck_Warning(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, pacificTime)
	// Spend $35 of $50 = 70%
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		35.00, now.Add(-1*time.Hour), "/a.jsonl", "local",
	)

	ba := alert.NewBudgetAlertWithClock(database, 50.00, func() time.Time { return now })
	status := ba.Check()

	if status.Level != alert.LevelYellow {
		t.Errorf("expected yellow at 70%%, got %v (pct=%f)", status.Level, status.Percent)
	}
}

func TestBudgetCheck_Critical(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, pacificTime)
	// Spend $45 of $50 = 90%
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		45.00, now.Add(-1*time.Hour), "/a.jsonl", "local",
	)

	ba := alert.NewBudgetAlertWithClock(database, 50.00, func() time.Time { return now })
	status := ba.Check()

	if status.Level != alert.LevelRed {
		t.Errorf("expected red at 90%%, got %v (pct=%f)", status.Level, status.Percent)
	}
}

func TestBudgetCheck_Exceeded(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, pacificTime)
	// Spend $60 of $50 = 120%
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		60.00, now.Add(-1*time.Hour), "/a.jsonl", "local",
	)

	ba := alert.NewBudgetAlertWithClock(database, 50.00, func() time.Time { return now })
	status := ba.Check()

	if !status.Exceeded {
		t.Error("should be exceeded")
	}
	if status.Remaining != 0 {
		t.Errorf("remaining should be 0 when exceeded, got %f", status.Remaining)
	}
}

func TestBudgetCheck_ZeroBudget(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	// $0 budget = disabled, should always be green
	ba := alert.NewBudgetAlert(database, 0)
	status := ba.Check()

	if status.Level != alert.LevelGreen {
		t.Errorf("zero budget should be green (disabled), got %v", status.Level)
	}
	if status.Exceeded {
		t.Error("zero budget should never be exceeded")
	}
}

func TestBudgetCheck_UsesLocalDayAcrossUTCMidnight(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Date(2026, 5, 10, 17, 30, 0, 0, pacificTime) // 2026-05-11 00:30 UTC
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		5.00, now.Add(-1*time.Hour), "/a.jsonl", "local", // Same local day, previous UTC day.
	)

	ba := alert.NewBudgetAlertWithClock(database, 50.00, func() time.Time { return now })
	status := ba.Check()

	if status.Spent < 4.99 || status.Spent > 5.01 {
		t.Errorf("expected local-day spent ~5.00 across UTC midnight, got %f", status.Spent)
	}
}
