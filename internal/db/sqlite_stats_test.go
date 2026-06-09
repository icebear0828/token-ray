package db_test

import (
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryCostSince(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	// Insert old record (2 days ago)
	old := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50}
	database.InsertUsageWithTime(old, 1.00, now.Add(-48*time.Hour), "/a.jsonl", "dev1")

	// Insert recent record (30 min ago)
	recent := model.TokenUsage{Source: "Claude Code", Model: "m2", InputTokens: 200, OutputTokens: 100}
	database.InsertUsageWithTime(recent, 2.00, now.Add(-30*time.Minute), "/b.jsonl", "dev1")

	// Insert very recent (5 min ago) on different device
	veryRecent := model.TokenUsage{Source: "Gemini CLI", Model: "m3", InputTokens: 300, OutputTokens: 150}
	database.InsertUsageWithTime(veryRecent, 3.00, now.Add(-5*time.Minute), "/c.jsonl", "dev2")

	// Last 1 hour, all devices = 5.00
	cost, _, _, _, _, _ := database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "", "")
	if cost < 4.99 || cost > 5.01 {
		t.Errorf("last 1h all: expected ~5.00, got %f", cost)
	}

	// Last 1 hour, dev1 only = 2.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "dev1", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("last 1h dev1: expected ~2.00, got %f", cost)
	}

	// Cumulative all = 6.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("", "")
	if cost < 5.99 || cost > 6.01 {
		t.Errorf("cumulative: expected ~6.00, got %f", cost)
	}

	// Cumulative dev2 = 3.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("dev2", "")
	if cost < 2.99 || cost > 3.01 {
		t.Errorf("cumulative dev2: expected ~3.00, got %f", cost)
	}
}
func TestQueryPeriodStatsBucketsMatchesIndividualQueries(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	rows := []struct {
		usage    model.TokenUsage
		cost     float64
		at       time.Time
		filePath string
		deviceID string
	}{
		{
			usage:    model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, CachedTokens: 10, CacheCreationTokens: 5, OutputTokens: 50},
			cost:     1.00,
			at:       now.Add(-48 * time.Hour),
			filePath: "/old.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Claude Code", Model: "m2", InputTokens: 200, CachedTokens: 20, CacheCreationTokens: 10, OutputTokens: 100},
			cost:     2.00,
			at:       now.Add(-30 * time.Minute),
			filePath: "/recent.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Gemini CLI", Model: "m3", InputTokens: 300, CachedTokens: 30, CacheCreationTokens: 15, OutputTokens: 150},
			cost:     3.00,
			at:       now.Add(-5 * time.Minute),
			filePath: "/other.jsonl",
			deviceID: "dev2",
		},
	}
	for _, row := range rows {
		if err := database.InsertUsageWithTime(row.usage, row.cost, row.at, row.filePath, row.deviceID); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.RawExec(
		"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		now.Format(time.RFC3339), "Claude Code", "ignored", 999, 999, 999, 999, 99.00, "/superseded.jsonl", "dev1", 1,
	); err != nil {
		t.Fatal(err)
	}

	windows := []db.PeriodStatsWindow{
		{Label: "1h", Since: now.Add(-1 * time.Hour)},
		{Label: "24h", Since: now.Add(-24 * time.Hour)},
		{Label: "ALL"},
	}
	buckets, err := database.QueryPeriodStatsBuckets(windows, "dev1", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != len(windows) {
		t.Fatalf("expected %d buckets, got %+v", len(windows), buckets)
	}

	for i, bucket := range buckets {
		var wantCost float64
		var wantInput, wantCached, wantCacheCreation, wantOutput int
		if windows[i].Since.IsZero() {
			wantCost, wantInput, wantCached, wantCacheCreation, wantOutput, err = database.QueryPeriodStatsAll("dev1", "Claude Code")
		} else {
			wantCost, wantInput, wantCached, wantCacheCreation, wantOutput, err = database.QueryPeriodStatsSince(windows[i].Since, "dev1", "Claude Code")
		}
		if err != nil {
			t.Fatal(err)
		}
		if bucket.Label != windows[i].Label || bucket.Cost != wantCost || bucket.InputTokens != wantInput || bucket.CachedTokens != wantCached || bucket.CacheCreationTokens != wantCacheCreation || bucket.OutputTokens != wantOutput {
			t.Fatalf("bucket %s mismatch: got %+v want cost=%f input=%d cached=%d cacheCreation=%d output=%d", windows[i].Label, bucket, wantCost, wantInput, wantCached, wantCacheCreation, wantOutput)
		}
	}
}
func TestQueryTokenSourceSummaries(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	for _, row := range []struct {
		usage    model.TokenUsage
		cost     float64
		at       time.Time
		filePath string
		deviceID string
	}{
		{
			usage:    model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
			cost:     1.00,
			at:       now.Add(-48 * time.Hour),
			filePath: "/old.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Claude Code", Model: "m2", InputTokens: 200, OutputTokens: 100},
			cost:     2.00,
			at:       now.Add(-30 * time.Minute),
			filePath: "/claude.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 400, OutputTokens: 50},
			cost:     4.00,
			at:       now.Add(-10 * time.Minute),
			filePath: "/codex.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 1000, OutputTokens: 100},
			cost:     5.00,
			at:       now.Add(-5 * time.Minute),
			filePath: "/other.jsonl",
			deviceID: "dev2",
		},
	} {
		if err := database.InsertUsageWithTime(row.usage, row.cost, row.at, row.filePath, row.deviceID); err != nil {
			t.Fatal(err)
		}
	}

	summaries, err := database.QueryTokenSourceSummaries(now.Add(-24*time.Hour), "dev1")
	if err != nil {
		t.Fatal(err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected two dev1 source summaries, got %+v", summaries)
	}
	if summaries[0].Source != "Claude Code" || summaries[0].Tokens24h != 300 || summaries[0].TokensTotal != 450 || summaries[0].CostTotal != 3.00 {
		t.Fatalf("unexpected Claude summary: %+v", summaries[0])
	}
	if summaries[1].Source != "Codex" || summaries[1].Tokens24h != 450 || summaries[1].TokensTotal != 450 || summaries[1].CostTotal != 4.00 {
		t.Fatalf("unexpected Codex summary: %+v", summaries[1])
	}
}

func TestQueryCostIntelligenceBuckets(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 30)
	rows := []struct {
		usage      model.TokenUsage
		cost       float64
		at         time.Time
		filePath   string
		deviceID   string
		superseded bool
	}{
		{
			usage:    model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 9000, CachedTokens: 9000, CacheCreationTokens: 9000, OutputTokens: 9000},
			cost:     99.00,
			at:       start.Add(-time.Second),
			filePath: "/before.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 100, CachedTokens: 40, CacheCreationTokens: 20, OutputTokens: 10},
			cost:     1.00,
			at:       start.Add(2 * time.Hour),
			filePath: "/codex-early.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 200, CachedTokens: 80, CacheCreationTokens: 40, OutputTokens: 20},
			cost:     2.00,
			at:       end.Add(-2 * time.Hour),
			filePath: "/codex-late.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 500, CachedTokens: 250, CacheCreationTokens: 100, OutputTokens: 50},
			cost:     3.00,
			at:       end.Add(-1 * time.Hour),
			filePath: "/claude-late.jsonl",
			deviceID: "dev1",
		},
		{
			usage:    model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 1000, OutputTokens: 100},
			cost:     5.00,
			at:       end.Add(-30 * time.Minute),
			filePath: "/other-device.jsonl",
			deviceID: "dev2",
		},
		{
			usage:      model.TokenUsage{Source: "Codex", Model: "ignored", InputTokens: 7000, OutputTokens: 700},
			cost:       7.00,
			at:         end.Add(-15 * time.Minute),
			filePath:   "/superseded.jsonl",
			deviceID:   "dev1",
			superseded: true,
		},
	}
	for _, row := range rows {
		if row.superseded {
			if err := database.RawExec(
				"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				row.at.Format(time.RFC3339Nano), row.usage.Source, row.usage.Model, row.usage.InputTokens, row.usage.CachedTokens, row.usage.CacheCreationTokens, row.usage.OutputTokens, row.cost, row.filePath, row.deviceID, 1,
			); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := database.InsertUsageWithTime(row.usage, row.cost, row.at, row.filePath, row.deviceID); err != nil {
			t.Fatal(err)
		}
	}

	dailyCosts, err := database.QueryDailyCostBuckets(start, end, "dev1", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(dailyCosts) != 2 {
		t.Fatalf("expected two non-empty Codex daily buckets, got %+v", dailyCosts)
	}
	if dailyCosts[0].Date != "2026-05-10" || dailyCosts[0].Cost != 1.00 {
		t.Fatalf("unexpected first daily cost bucket: %+v", dailyCosts[0])
	}
	if dailyCosts[1].Date != "2026-06-08" || dailyCosts[1].Cost != 2.00 {
		t.Fatalf("unexpected second daily cost bucket: %+v", dailyCosts[1])
	}

	heatmap, err := database.QueryCalendarHeatmapBuckets(start, end, "dev1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(heatmap) != 2 {
		t.Fatalf("expected two non-empty heatmap buckets, got %+v", heatmap)
	}
	if heatmap[0].Date != "2026-05-10" || heatmap[0].Tokens != 110 || heatmap[0].Events != 1 || heatmap[0].Cost != 1.00 {
		t.Fatalf("unexpected first heatmap bucket: %+v", heatmap[0])
	}
	if heatmap[1].Date != "2026-06-08" || heatmap[1].Tokens != 770 || heatmap[1].Events != 2 || heatmap[1].Cost != 5.00 {
		t.Fatalf("unexpected second heatmap bucket: %+v", heatmap[1])
	}

	toolShare, err := database.QueryToolShareBuckets(start, end, "dev1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(toolShare) != 2 {
		t.Fatalf("expected two tool-share buckets, got %+v", toolShare)
	}
	if toolShare[0].Source != "Claude Code" || toolShare[0].Tokens != 550 || toolShare[0].Events != 1 || toolShare[0].TotalCost != 3.00 {
		t.Fatalf("unexpected first tool-share bucket: %+v", toolShare[0])
	}
	if toolShare[1].Source != "Codex" || toolShare[1].Tokens != 330 || toolShare[1].Events != 2 || toolShare[1].TotalCost != 3.00 {
		t.Fatalf("unexpected second tool-share bucket: %+v", toolShare[1])
	}
}
func TestQueryStatsSince(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 1000, OutputTokens: 400},
		3.00, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-1*time.Minute), "/c.jsonl", "local",
	)

	stats, err := database.QueryStatsSince(now.Add(-1*time.Hour), "", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 model groups, got %d", len(stats))
	}

	// Stats should be sorted by cost descending
	if stats[0].Model != "claude-opus-4-7" {
		t.Errorf("expected first model claude-opus-4-7, got %s", stats[0].Model)
	}
	if stats[0].TotalCost < 4.49 || stats[0].TotalCost > 4.51 {
		t.Errorf("expected claude cost ~4.50, got %f", stats[0].TotalCost)
	}
	if stats[0].Events != 2 {
		t.Errorf("expected 2 events, got %d", stats[0].Events)
	}
}
func TestQuerySinceHandlesMixedTimestampPrecision(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	err = database.RawExec(
		"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"2026-05-07T11:13:02Z", "Claude Code", "claude-opus-4-7", 200, 0, 0, 20, 2.00, "/old.jsonl", "local",
	)
	if err != nil {
		t.Fatal(err)
	}
	err = database.RawExec(
		"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"2026-05-07T11:13:03.316Z", "Gemini CLI", "gemini-2.5-pro", 100, 0, 0, 10, 1.00, "/new.jsonl", "local",
	)
	if err != nil {
		t.Fatal(err)
	}

	since := time.Date(2026, 5, 7, 11, 13, 3, 0, time.UTC)
	cost, input, _, _, output, err := database.QueryPeriodStatsSince(since, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 1.00 || input != 100 || output != 10 {
		t.Fatalf("expected fractional same-second row only, got cost=%f input=%d output=%d", cost, input, output)
	}

	records, err := database.QuerySyncRecordsSince(time.Now().UTC().Add(-1 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected sync records to use updated_at cursor, got %+v", records)
	}
	var foundGemini bool
	for _, record := range records {
		if record.Timestamp.IsZero() || record.UpdatedAt.IsZero() {
			t.Fatalf("expected parsed sync timestamps, got %+v", records)
		}
		if record.Source == "Gemini CLI" {
			foundGemini = true
		}
	}
	if !foundGemini {
		t.Fatalf("expected parsed fractional Gemini sync record, got %+v", records)
	}
}
func TestSupersededRowsAreExcludedFromDefaultQueries(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	path := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 1000, OutputTokens: 300},
		1.00, now, path, "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 2000, OutputTokens: 400, UUID: "gemini:active"},
		2.00, now.Add(time.Second), path, "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 3000, OutputTokens: 500},
		3.00, now, "/Users/c/.gemini/tmp/old/chats/session.jsonl", "old-device",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{path, "/Users/c/.gemini/tmp/old/chats/session.jsonl"}, []string{"local", "old-device"}); err != nil {
		t.Fatal(err)
	}

	cost, input, _, _, output, err := database.QueryPeriodStatsAll("", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 2.00 || input != 2000 || output != 400 {
		t.Fatalf("expected only active Gemini row in stats, got cost=%f input=%d output=%d", cost, input, output)
	}

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0] != "local" {
		t.Fatalf("expected only active local device, got %+v", devices)
	}

	usageRecords, err := database.QueryUsageRecords(time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(usageRecords) != 1 || usageRecords[0].InputTokens != 2000 {
		t.Fatalf("expected one active usage record, got %+v", usageRecords)
	}

	syncRecords, err := database.QuerySyncRecordsSince(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(syncRecords) != 3 {
		t.Fatalf("expected active and superseded sync records, got %+v", syncRecords)
	}
	var active, superseded int
	for _, r := range syncRecords {
		if r.Superseded {
			superseded++
		} else {
			active++
		}
		if r.UpdatedAt.IsZero() {
			t.Fatalf("expected sync record updated_at, got %+v", r)
		}
	}
	if active != 1 || superseded != 2 {
		t.Fatalf("expected 1 active and 2 superseded sync records, active=%d superseded=%d records=%+v", active, superseded, syncRecords)
	}
}
func TestQueryStatsSourceFilters(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "api", InputTokens: 1000, OutputTokens: 200},
		1.50, now, "/claude.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now, "/codex.sqlite", "local",
	)

	stats, err := database.QueryStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Source != "Codex" || stats[0].Model != "gpt-5.5" {
		t.Fatalf("expected only Codex model stats, got %+v", stats)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "dashboard" {
		t.Fatalf("expected only Codex project stats, got %+v", projects)
	}
}
