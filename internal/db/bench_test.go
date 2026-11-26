package db_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

func seedBenchDB(b *testing.B, rowCount int) (*db.DB, time.Time) {
	b.Helper()
	database, err := db.New(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("open bench db: %v", err)
	}
	b.Cleanup(func() { _ = database.Close() })

	now := time.Now().UTC()
	devices := []string{"local", "mac-studio", "linux-worker"}
	sources := []string{"Claude Code", "Codex", "Gemini CLI", "DeepSeek Harness"}
	models := []string{
		"claude-opus-4-7",
		"claude-sonnet-4-6",
		"gpt-5.5",
		"gpt-5.4-mini",
		"gemini-2.5-pro",
		"gemini-3-flash",
		"deepseek-chat",
		"deepseek-reasoner",
	}
	projects := []string{
		"token-ray", "agent-core", "web-ui", "infra", "cli",
		"evals", "docs", "sandbox", "ml-pipeline", "gateway",
	}

	err = database.RunInTx(func(tx *db.Tx) error {
		for i := 0; i < rowCount; i++ {
			age := time.Duration(i%8760) * time.Hour // up to 365 days
			if i%20 == 0 {
				age = time.Duration(i%45) * time.Minute // recent 1h rows
			}
			ts := now.Add(-age)
			src := sources[i%len(sources)]
			mdl := models[i%len(models)]
			proj := projects[i%len(projects)]
			dev := devices[i%len(devices)]

			var durMS int64
			var timedOut int
			outTok := 100 + (i%50)*20
			if i%3 == 0 {
				durMS = int64(800 + (i%10)*250)
				timedOut = outTok
			}

			u := model.TokenUsage{
				Source:              src,
				Model:               mdl,
				Project:             proj,
				InputTokens:         1000 + (i%20)*250,
				CachedTokens:        400 + (i%10)*100,
				CacheCreationTokens: 50 + (i%5)*25,
				OutputTokens:        outTok,
				DurationMS:          durMS,
				TimedOutputTokens:   timedOut,
				UUID:                fmt.Sprintf("bench-uuid-%d", i),
			}
			cost := float64(u.InputTokens)*0.000003 + float64(u.OutputTokens)*0.000015
			if err := tx.InsertUsageWithTime(u, cost, ts, fmt.Sprintf("/tmp/%s/%s/session-%d.jsonl", dev, proj, i%40), dev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatalf("seed bench rows: %v", err)
	}

	// Add 1,000 superseded rows so WHERE COALESCE(superseded, 0) = 0 exercises filtering.
	if err := database.RawExec(`
		WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 1000)
		INSERT INTO usage_records (log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, uuid, superseded)
		SELECT ?, 'Claude Code', 'claude-opus-4-7', 'superseded-proj', 500, 100, 0, 50, 0.5, '/tmp/sup.jsonl', 'local', 'sup-' || x, 1
		FROM cnt
	`, now.Add(-2*time.Hour).Format(time.RFC3339Nano)); err != nil {
		b.Fatalf("seed superseded rows: %v", err)
	}

	return database, now
}

func benchmarkWindows(now time.Time) []db.PeriodStatsWindow {
	return []db.PeriodStatsWindow{
		{Label: "1h", Since: now.Add(-1 * time.Hour)},
		{Label: "24h", Since: now.Add(-24 * time.Hour)},
		{Label: "7d", Since: now.Add(-7 * 24 * time.Hour)},
		{Label: "30d", Since: now.Add(-30 * 24 * time.Hour)},
		{Label: "3mo", Since: now.Add(-90 * 24 * time.Hour)},
		{Label: "6mo", Since: now.Add(-180 * 24 * time.Hour)},
		{Label: "1y", Since: now.Add(-365 * 24 * time.Hour)},
		{Label: "ALL"},
	}
}

func BenchmarkQueryPeriodStatsBuckets(b *testing.B) {
	database, now := seedBenchDB(b, 20000)
	windows := benchmarkWindows(now)

	b.Run("AllDevices", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buckets, err := database.QueryPeriodStatsBuckets(windows, "", "")
			if err != nil {
				b.Fatal(err)
			}
			if len(buckets) != len(windows) {
				b.Fatalf("got %d buckets, want %d", len(buckets), len(windows))
			}
		}
	})

	b.Run("FilteredDeviceAndSource", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buckets, err := database.QueryPeriodStatsBuckets(windows, "local", "Claude Code")
			if err != nil {
				b.Fatal(err)
			}
			if len(buckets) != len(windows) {
				b.Fatalf("got %d buckets, want %d", len(buckets), len(windows))
			}
		}
	})
}

func BenchmarkQueryStatsSince(b *testing.B) {
	database, now := seedBenchDB(b, 20000)
	since := now.Add(-30 * 24 * time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats, err := database.QueryStatsSince(since, "", "")
		if err != nil {
			b.Fatal(err)
		}
		if len(stats) == 0 {
			b.Fatal("expected non-empty stats")
		}
	}
}

func BenchmarkQueryProjectStatsSince(b *testing.B) {
	database, now := seedBenchDB(b, 20000)
	since := now.Add(-30 * 24 * time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		projects, err := database.QueryProjectStatsSince(since, "", "")
		if err != nil {
			b.Fatal(err)
		}
		if len(projects) == 0 {
			b.Fatal("expected non-empty projects")
		}
	}
}
