package web_test

import (
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIStats(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude records
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 5000, OutputTokens: 200},
		1.50, now.Add(-30*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 8000, OutputTokens: 400},
		3.00, now.Add(-2*time.Hour), "/b.jsonl", "local",
	)
	// Gemini records
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-10*time.Minute), "/c.jsonl", "local",
	)

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.StatsResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// Should have time periods
	if len(data.Periods) == 0 {
		t.Fatal("expected periods")
	}

	// Should have sources grouped
	if len(data.Sources) < 2 {
		t.Fatalf("expected at least 2 sources, got %d", len(data.Sources))
	}

	// Find Claude source
	var claude *model.SourceStats
	for i := range data.Sources {
		if data.Sources[i].Name == "Claude Code" {
			claude = &data.Sources[i]
		}
	}
	if claude == nil {
		t.Fatal("Claude Code source not found")
	}
	if claude.TotalInput != 3000 || claude.TotalCached != 13000 || claude.TotalOutput != 600 {
		t.Errorf("Claude totals wrong: %+v", claude)
	}
	if len(claude.Models) != 1 {
		t.Errorf("expected 1 Claude model, got %d", len(claude.Models))
	}
	if len(claude.Models) == 1 {
		modelStats := claude.Models[0]
		if modelStats.InputTokens != 3000 || modelStats.CachedTokens != 13000 || modelStats.OutputTokens != 600 {
			t.Errorf("Claude model token breakdown missing/wrong: %+v", modelStats)
		}
		if modelStats.CacheCreationPricePerM != 6.25 {
			t.Errorf("Claude model cache creation price missing/wrong: %+v", modelStats)
		}
	}

	// Should have devices list
	if len(data.Devices) == 0 {
		t.Error("expected devices list")
	}
}
func TestAPIStatsCodexSourceFilter(t *testing.T) {
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

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(srv.URL + "/api/stats?device=all&source=Codex")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Periods) != 8 {
		t.Fatalf("expected 8 periods, got %d", len(data.Periods))
	}
	if len(data.Sources) != 1 || data.Sources[0].Name != "Codex" {
		t.Fatalf("expected only Codex source, got %+v", data.Sources)
	}
	if len(data.Projects) != 1 || data.Projects[0].Project != "dashboard" {
		t.Fatalf("expected only Codex project, got %+v", data.Projects)
	}
	if data.Sources[0].TotalInput != 2000 || data.Sources[0].TotalCached != 1500 || data.Sources[0].TotalOutput != 300 {
		t.Fatalf("unexpected Codex token totals: %+v", data.Sources[0])
	}
}
func TestAPIStatsDetailModes(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now.Add(-1*time.Minute), "/codex.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	summaryResp, err := http.Get(srv.URL + "/api/stats?device=all&source=Codex&detail=summary")
	if err != nil {
		t.Fatal(err)
	}
	defer summaryResp.Body.Close()
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected summary 200, got %d", summaryResp.StatusCode)
	}
	var summary model.StatsResponse
	if err := json.NewDecoder(summaryResp.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Periods) == 0 || len(summary.Devices) == 0 {
		t.Fatalf("expected summary periods/devices, got %+v", summary)
	}
	if len(summary.Sources) != 1 || len(summary.Sources[0].Models) != 0 || len(summary.Projects) != 0 {
		t.Fatalf("summary should include source totals only, got %+v", summary)
	}
	if len(summary.Charts.DailyCosts) != 30 || len(summary.Charts.CalendarHeatmap) != 30 {
		t.Fatalf("summary should include cost intelligence charts, got %+v", summary.Charts)
	}
	if len(summary.Charts.ToolShare) != 1 || summary.Charts.ToolShare[0].Source != "Codex" || summary.Charts.ToolShare[0].Tokens != 2300 {
		t.Fatalf("summary should include Codex token-share chart data, got %+v", summary.Charts.ToolShare)
	}

	detailsResp, err := http.Get(srv.URL + "/api/stats?device=all&source=Codex&detail=details")
	if err != nil {
		t.Fatal(err)
	}
	defer detailsResp.Body.Close()
	if detailsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected details 200, got %d", detailsResp.StatusCode)
	}
	var details model.StatsResponse
	if err := json.NewDecoder(detailsResp.Body).Decode(&details); err != nil {
		t.Fatal(err)
	}
	if len(details.Periods) != 0 || len(details.Devices) != 0 {
		t.Fatalf("details should omit summary periods/devices, got %+v", details)
	}
	if len(details.Charts.DailyCosts) != 0 || len(details.Charts.CalendarHeatmap) != 0 || len(details.Charts.ToolShare) != 0 {
		t.Fatalf("details should omit cost intelligence charts, got %+v", details.Charts)
	}
	if len(details.Sources) != 1 || len(details.Sources[0].Models) != 1 || len(details.Projects) != 1 {
		t.Fatalf("expected details models/projects, got %+v", details)
	}

	badResp, err := http.Get(srv.URL + "/api/stats?detail=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid detail 400, got %d", badResp.StatusCode)
	}
}
func TestAPICacheSavings(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude: 1000 input (800 cached), 200 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 800, OutputTokens: 200},
		0, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	// Gemini: 500 input (0 cached), 100 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/cache-savings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.CacheSavingsResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// actual_cost: calculator computes real cost with cached pricing
	// hypothetical_cost: calculator computes cost as if cached_tokens were charged at full input price
	if data.HypotheticalCost <= data.ActualCost {
		t.Errorf("hypothetical should exceed actual: hypo=%f actual=%f", data.HypotheticalCost, data.ActualCost)
	}
	if data.Saved < 0 {
		t.Errorf("saved should be >= 0, got %f", data.Saved)
	}
	if data.CacheHitRate < 0 || data.CacheHitRate > 100 {
		t.Errorf("cache hit rate should be 0-100, got %f", data.CacheHitRate)
	}
}
