package dashboard_test

import (
	"testing"
	"time"

	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func TestBuildCacheSavings(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	usage := model.TokenUsage{
		Source:              "Claude Code",
		Model:               "claude-opus-4-7",
		InputTokens:         1000,
		CachedTokens:        800,
		CacheCreationTokens: 100,
		OutputTokens:        200,
	}
	if err := database.InsertUsageWithTime(usage, 0, now.Add(-time.Minute), "/local.jsonl", "local"); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(usage, 0, now.Add(-time.Minute), "/remote.jsonl", "remote"); err != nil {
		t.Fatal(err)
	}

	savings, err := dashboard.BuildCacheSavings(database, calc, "local")
	if err != nil {
		t.Fatal(err)
	}

	// Actual: 100 base input * $5/M + 800 cached * $0.5/M + 100 cache
	// creation * $6.25/M + 200 output * $25/M.
	assertApprox(t, savings.ActualCost, 0.006525)
	// Hypothetical: all 1000 input tokens at $5/M plus 200 output at $25/M.
	assertApprox(t, savings.HypotheticalCost, 0.01)
	assertApprox(t, savings.Saved, 0.003475)
	assertApprox(t, savings.SavedPercent, 34.75)
	assertApprox(t, savings.CacheHitRate, 80)
}

func TestBuildCacheSavingsAllDevices(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	usage := model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200}
	if err := database.InsertUsageWithTime(usage, 0, now.Add(-time.Minute), "/local.jsonl", "local"); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(usage, 0, now.Add(-time.Minute), "/remote.jsonl", "remote"); err != nil {
		t.Fatal(err)
	}

	all, err := dashboard.BuildCacheSavings(database, calc, "all")
	if err != nil {
		t.Fatal(err)
	}
	local, err := dashboard.BuildCacheSavings(database, calc, "local")
	if err != nil {
		t.Fatal(err)
	}

	assertApprox(t, all.ActualCost, local.ActualCost*2)
}
