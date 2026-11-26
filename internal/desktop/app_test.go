package desktop_test

import (
	"testing"
	"time"

	"ai-flight-dashboard/internal/desktop"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
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

func TestGetPricingIncludesCacheCreationPrice(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	app := desktop.NewApp(database, calc)
	entries, err := app.GetPricing()
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.Model == "claude-opus-4-7" {
			if entry.CacheCreationPricePerM != 6.25 {
				t.Fatalf("expected cache creation price 6.25, got %v", entry.CacheCreationPricePerM)
			}
			return
		}
	}
	t.Fatal("expected claude-opus-4-7 pricing entry")
}

func TestSetDeviceAliasInvalidatesStatsCache(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
		1.50, now.Add(-time.Minute), "/claude.jsonl", "nas.local",
	); err != nil {
		t.Fatal(err)
	}

	app := desktop.NewApp(database, calc)

	initial, err := app.GetStats("all", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Devices) != 1 || initial.Devices[0].DisplayName != "nas.local" {
		t.Fatalf("expected initial display name nas.local, got %+v", initial.Devices)
	}

	if err := app.SetDeviceAlias("nas.local", "Studio NAS"); err != nil {
		t.Fatal(err)
	}

	updated, err := app.GetStats("all", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Devices) != 1 || updated.Devices[0].DisplayName != "Studio NAS" {
		t.Fatalf("expected stats cache invalidated with Studio NAS display name, got %+v", updated.Devices)
	}
}
