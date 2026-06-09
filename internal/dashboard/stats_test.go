package dashboard_test

import (
	"math"
	"testing"
	"time"

	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func TestBuildStatsIncludesCacheHitRates(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()
	calc := testutil.NewTestCalc(t)

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:              "Codex",
			Model:               "gpt-5.5",
			Project:             "token",
			InputTokens:         1000,
			CachedTokens:        250,
			CacheCreationTokens: 100,
			OutputTokens:        50,
		},
		1.00,
		now.Add(-10*time.Minute),
		"/codex.sqlite",
		"local",
	); err != nil {
		t.Fatal(err)
	}

	stats, err := dashboard.BuildStats(database, calc, "local", "Codex", false)
	if err != nil {
		t.Fatal(err)
	}

	allPeriod := stats.Periods[len(stats.Periods)-1]
	assertApprox(t, stats.Periods[0].CacheHitRate, 25.0)
	assertApprox(t, allPeriod.CacheHitRate, 25.0)
	if len(stats.Sources) != 1 {
		t.Fatalf("expected one source, got %+v", stats.Sources)
	}
	assertApprox(t, stats.Sources[0].CacheHitRate, 25.0)
	if len(stats.Sources[0].Models) != 1 {
		t.Fatalf("expected one model, got %+v", stats.Sources[0].Models)
	}
	assertApprox(t, stats.Sources[0].Models[0].CacheHitRate, 25.0)
	if len(stats.Projects) != 1 {
		t.Fatalf("expected one project, got %+v", stats.Projects)
	}
	assertApprox(t, stats.Projects[0].CacheHitRate, 25.0)
}

func TestBuildStatsSummaryOmitsHeavyDetails(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()
	calc := testutil.NewTestCalc(t)

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Codex",
			Model:        "gpt-5.5",
			Project:      "token",
			InputTokens:  1000,
			CachedTokens: 250,
			OutputTokens: 50,
		},
		1.00,
		now.Add(-10*time.Minute),
		"/codex.sqlite",
		"local",
	); err != nil {
		t.Fatal(err)
	}

	summary, err := dashboard.BuildStatsSummary(database, calc, "local", "Codex", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(summary.Periods) != 8 {
		t.Fatalf("expected period summary, got %+v", summary.Periods)
	}
	if len(summary.Sources) != 1 {
		t.Fatalf("expected one source total, got %+v", summary.Sources)
	}
	if summary.Sources[0].Name != "Codex" || summary.Sources[0].TotalInput != 1000 || summary.Sources[0].TotalCached != 250 {
		t.Fatalf("unexpected source summary: %+v", summary.Sources[0])
	}
	if len(summary.Sources[0].Models) != 0 {
		t.Fatalf("summary should omit model details, got %+v", summary.Sources[0].Models)
	}
	if len(summary.Projects) != 0 {
		t.Fatalf("summary should omit project details, got %+v", summary.Projects)
	}
	if len(summary.Devices) != 1 || summary.Devices[0].ID != "local" {
		t.Fatalf("expected devices in summary, got %+v", summary.Devices)
	}
}

func TestBuildStatsSummaryIncludesCostIntelligenceCharts(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()
	calc := testutil.NewTestCalc(t)

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:              "Codex",
			Model:               "gpt-5.5",
			Project:             "token",
			InputTokens:         1000,
			CachedTokens:        600,
			CacheCreationTokens: 100,
			OutputTokens:        50,
		},
		1.25,
		now,
		"/codex.sqlite",
		"local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Claude Code",
			Model:        "claude-opus-4-7",
			Project:      "token",
			InputTokens:  2000,
			OutputTokens: 100,
		},
		2.50,
		now,
		"/claude.jsonl",
		"local",
	); err != nil {
		t.Fatal(err)
	}

	summary, err := dashboard.BuildStatsSummary(database, calc, "local", "Codex", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(summary.Charts.DailyCosts) != 30 {
		t.Fatalf("expected 30 daily cost buckets, got %+v", summary.Charts.DailyCosts)
	}
	if len(summary.Charts.CalendarHeatmap) != 30 {
		t.Fatalf("expected 30 heatmap buckets, got %+v", summary.Charts.CalendarHeatmap)
	}
	today := now.Format("2006-01-02")
	var todayDailyCost *model.DailyCostBucket
	for i := range summary.Charts.DailyCosts {
		if summary.Charts.DailyCosts[i].Date == today {
			todayDailyCost = &summary.Charts.DailyCosts[i]
			break
		}
	}
	if todayDailyCost == nil || todayDailyCost.Cost != 1.25 {
		t.Fatalf("expected today Codex cost 1.25, got %+v", todayDailyCost)
	}
	var todayHeatmap *model.CalendarHeatmapBucket
	for i := range summary.Charts.CalendarHeatmap {
		if summary.Charts.CalendarHeatmap[i].Date == today {
			todayHeatmap = &summary.Charts.CalendarHeatmap[i]
			break
		}
	}
	if todayHeatmap == nil || todayHeatmap.Tokens != 1050 || todayHeatmap.Events != 1 || todayHeatmap.Cost != 1.25 {
		t.Fatalf("expected today Codex heatmap bucket, got %+v", todayHeatmap)
	}
	if len(summary.Charts.ToolShare) != 1 || summary.Charts.ToolShare[0].Source != "Codex" || summary.Charts.ToolShare[0].Tokens != 1050 {
		t.Fatalf("expected Codex-only tool share by total tokens, got %+v", summary.Charts.ToolShare)
	}
}

func TestBuildStatsDetailsOmitsSummaryData(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()
	calc := testutil.NewTestCalc(t)

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Codex",
			Model:        "gpt-5.5",
			Project:      "token",
			InputTokens:  1000,
			CachedTokens: 250,
			OutputTokens: 50,
		},
		1.00,
		now.Add(-10*time.Minute),
		"/codex.sqlite",
		"local",
	); err != nil {
		t.Fatal(err)
	}

	details, err := dashboard.BuildStatsDetails(database, calc, "local", "Codex", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(details.Periods) != 0 || len(details.Devices) != 0 {
		t.Fatalf("details should omit period/device summary data, got periods=%+v devices=%+v", details.Periods, details.Devices)
	}
	if len(details.Charts.DailyCosts) != 0 || len(details.Charts.CalendarHeatmap) != 0 || len(details.Charts.ToolShare) != 0 {
		t.Fatalf("details should omit cost intelligence charts, got %+v", details.Charts)
	}
	if len(details.Sources) != 1 || len(details.Sources[0].Models) != 1 {
		t.Fatalf("expected source model details, got %+v", details.Sources)
	}
	if len(details.Projects) != 1 || details.Projects[0].Project != "token" {
		t.Fatalf("expected project details, got %+v", details.Projects)
	}
}

func TestBuildTokenSummaryIncludesPerSourceBreakdown(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	for _, row := range []struct {
		usage  model.TokenUsage
		cost   float64
		device string
	}{
		{
			usage:  model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
			cost:   1.20,
			device: "remote",
		},
		{
			usage:  model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 3000, OutputTokens: 400},
			cost:   3.40,
			device: "remote",
		},
		{
			usage:  model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 5000, OutputTokens: 600},
			cost:   5.60,
			device: "other",
		},
	} {
		if err := database.InsertUsageWithTime(row.usage, row.cost, now, "/session.jsonl", row.device); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := dashboard.BuildTokenSummary(database, "remote")
	if err != nil {
		t.Fatal(err)
	}

	if summary.TokensTotal != 4600 || summary.Tokens24h != 4600 || summary.CostTotal != 4.60 {
		t.Fatalf("unexpected total summary: %+v", summary)
	}
	if len(summary.Sources) != 2 {
		t.Fatalf("expected two remote source summaries, got %+v", summary.Sources)
	}
	if summary.Sources[0].Source != "Claude Code" || summary.Sources[0].TokensTotal != 1200 {
		t.Fatalf("unexpected first source summary: %+v", summary.Sources)
	}
	if summary.Sources[1].Source != "Codex" || summary.Sources[1].TokensTotal != 3400 {
		t.Fatalf("unexpected second source summary: %+v", summary.Sources)
	}
}

func assertApprox(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("expected %.3f, got %.3f", want, got)
	}
}
