package db_test

import (
	"token-ray/internal/db"
	"token-ray/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestRebuildSourceTotalsSummaryMatchesRawTotals(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	rows := []struct {
		usage      model.TokenUsage
		cost       float64
		at         time.Time
		filePath   string
		deviceID   string
		superseded int
	}{
		{
			usage:    model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, CachedTokens: 20, CacheCreationTokens: 5, OutputTokens: 50},
			cost:     1.00,
			at:       now.Add(-3 * time.Hour),
			filePath: "/claude-a.jsonl",
			deviceID: "mac",
		},
		{
			usage:    model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 200, CachedTokens: 80, CacheCreationTokens: 10, OutputTokens: 60},
			cost:     2.00,
			at:       now.Add(-2 * time.Hour),
			filePath: "/codex-mac.sqlite",
			deviceID: "mac",
		},
		{
			usage:    model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 300, CachedTokens: 120, CacheCreationTokens: 15, OutputTokens: 70},
			cost:     3.00,
			at:       now.Add(-1 * time.Hour),
			filePath: "/codex-linux.sqlite",
			deviceID: "linux",
		},
		{
			usage:      model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 999, CachedTokens: 999, CacheCreationTokens: 999, OutputTokens: 999},
			cost:       9.99,
			at:         now,
			filePath:   "/superseded.sqlite",
			deviceID:   "mac",
			superseded: 1,
		},
	}
	for _, row := range rows {
		if err := database.RawExec(
			"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			row.at.Format(time.RFC3339Nano), row.usage.Source, row.usage.Model, row.usage.InputTokens, row.usage.CachedTokens, row.usage.CacheCreationTokens, row.usage.OutputTokens, row.cost, row.filePath, row.deviceID, row.superseded,
		); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.RebuildSourceTotalsSummary(); err != nil {
		t.Fatal(err)
	}

	rawAll, err := database.QuerySourceTotalsSince(now.Add(-24*time.Hour), "", "")
	if err != nil {
		t.Fatal(err)
	}
	summaryAll, err := database.QuerySourceTotalsSummary("", "")
	if err != nil {
		t.Fatal(err)
	}
	assertSourceTotalsEqual(t, summaryAll, rawAll)

	rawMacCodex, err := database.QuerySourceTotalsSince(now.Add(-24*time.Hour), "mac", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	summaryMacCodex, err := database.QuerySourceTotalsSummary("mac", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	assertSourceTotalsEqual(t, summaryMacCodex, rawMacCodex)
}
func TestSourceTotalsSummaryTracksInsertUpdateAndSupersede(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	usage := model.TokenUsage{
		Source:       "Codex",
		Model:        "gpt-5.5",
		InputTokens:  100,
		CachedTokens: 40,
		OutputTokens: 20,
		UUID:         "codex-session:1",
	}
	if err := database.InsertUsageWithTime(usage, 1.00, now, "/codex.sqlite", "mac"); err != nil {
		t.Fatal(err)
	}
	assertSingleSourceTotal(t, database, "mac", "Codex", 1, 100, 40, 0, 20, 1.00)

	usage.InputTokens = 250
	usage.CachedTokens = 125
	usage.CacheCreationTokens = 30
	usage.OutputTokens = 50
	if err := database.InsertUsageWithTime(usage, 2.50, now.Add(time.Minute), "/codex.sqlite", "mac"); err != nil {
		t.Fatal(err)
	}
	assertSingleSourceTotal(t, database, "mac", "Codex", 1, 250, 125, 30, 50, 2.50)

	if _, err := database.SupersedeUsageBySourceFilePathDeviceUUIDPrefix("Codex", "/codex.sqlite", "mac", "codex-session:"); err != nil {
		t.Fatal(err)
	}
	stats, err := database.QuerySourceTotalsSummary("mac", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Fatalf("expected superseded Codex row to be removed from summary, got %+v", stats)
	}
}
func TestSourceTotalsSummaryIgnoresNonAggregateUpdates(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Codex",
			Model:        "gpt-5.5",
			Project:      "before",
			InputTokens:  100,
			CachedTokens: 40,
			OutputTokens: 20,
			UUID:         "codex-session:aggregate-stable",
		},
		1.00,
		now,
		"/codex.sqlite",
		"mac",
	); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`CREATE TRIGGER fail_source_totals_insert BEFORE INSERT ON usage_source_totals BEGIN SELECT RAISE(ABORT, 'source totals insert touched'); END`,
		`CREATE TRIGGER fail_source_totals_update BEFORE UPDATE ON usage_source_totals BEGIN SELECT RAISE(ABORT, 'source totals update touched'); END`,
		`CREATE TRIGGER fail_source_totals_delete BEFORE DELETE ON usage_source_totals BEGIN SELECT RAISE(ABORT, 'source totals delete touched'); END`,
	} {
		if err := database.RawExec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.RawExec("UPDATE usage_records SET project = ? WHERE uuid = ?", "after", "codex-session:aggregate-stable"); err != nil {
		t.Fatalf("project-only update should not touch source totals summary: %v", err)
	}
	assertSingleSourceTotal(t, database, "mac", "Codex", 1, 100, 40, 0, 20, 1.00)
}
