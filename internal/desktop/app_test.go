package desktop_test

import (
	"testing"
	"time"

	"token-ray/internal/desktop"
	"token-ray/internal/model"
	"token-ray/internal/testutil"
)

func TestGetStatsFiltersBySource(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "api", InputTokens: 1000, OutputTokens: 200},
		1.50, now.Add(-2*time.Minute), "/claude.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now.Add(-1*time.Minute), "/codex.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	app := desktop.NewApp(database, calc)
	stats, err := app.GetStats("all", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Sources) != 1 || stats.Sources[0].Name != "Codex" {
		t.Fatalf("expected only Codex source, got %+v", stats.Sources)
	}
	if len(stats.Projects) != 1 || stats.Projects[0].Project != "dashboard" {
		t.Fatalf("expected only Codex project, got %+v", stats.Projects)
	}
	if len(stats.Periods) != 8 {
		t.Fatalf("expected 8 periods, got %d", len(stats.Periods))
	}
}
