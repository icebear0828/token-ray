package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

func TestTimingCountersAggregateAndSync(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	records := []model.TokenUsage{
		{
			Source: "Claude Code", Model: "claude-opus-4-7", Project: "alpha",
			InputTokens: 10, OutputTokens: 100, DurationMS: 2000, TimedOutputTokens: 100,
			UUID: "timed-1",
		},
		{
			Source: "Claude Code", Model: "claude-opus-4-7", Project: "alpha",
			InputTokens: 10, OutputTokens: 50,
			UUID: "untimed-1",
		},
		{
			Source: "Claude Code", Model: "claude-opus-4-7", Project: "alpha",
			InputTokens: 10, OutputTokens: 40, DurationMS: 1000, TimedOutputTokens: 999,
			UUID: "timed-2",
		},
	}
	for i, record := range records {
		record.Timestamp = now.Add(time.Duration(i-3) * time.Minute)
		if err := database.InsertUsageWithTime(record, 0, record.Timestamp, "/log.jsonl", "dev"); err != nil {
			t.Fatal(err)
		}
	}

	// A re-scan that predates timing must not wipe an existing sample.
	if err := database.InsertUsageWithTime(model.TokenUsage{
		Source: "Claude Code", Model: "claude-opus-4-7", Project: "alpha",
		InputTokens: 10, OutputTokens: 100, UUID: "timed-1",
	}, 0, now.Add(-3*time.Minute), "/log.jsonl", "dev"); err != nil {
		t.Fatal(err)
	}

	buckets, err := database.QueryPeriodStatsBuckets([]db.PeriodStatsWindow{{Label: "ALL"}}, "dev", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 {
		t.Fatalf("expected one bucket, got %+v", buckets)
	}
	bucket := buckets[0]
	if bucket.OutputTokens != 190 || bucket.TimedOutputTokens != 140 || bucket.DurationMS != 3000 || bucket.TimedEvents != 2 {
		t.Fatalf("unexpected period timing: %+v", bucket)
	}

	modelStats, err := database.QueryStatsSince(time.Time{}, "dev", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if len(modelStats) != 1 || modelStats[0].TimedOutputTokens != 140 || modelStats[0].DurationMS != 3000 || modelStats[0].TimedEvents != 2 {
		t.Fatalf("unexpected model timing: %+v", modelStats)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "dev", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].TimedOutputTokens != 140 || projects[0].DurationMS != 3000 || projects[0].TimedEvents != 2 {
		t.Fatalf("unexpected project timing: %+v", projects)
	}

	page, err := database.QuerySyncRecordsPage(time.Time{}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 3 {
		t.Fatalf("expected three sync records, got %+v", page.Records)
	}
	for _, record := range page.Records {
		switch record.UUID {
		case "timed-1":
			if record.DurationMS != 2000 || record.TimedOutputTokens != 100 {
				t.Fatalf("timed-1 lost timing: %+v", record)
			}
		case "timed-2":
			if record.DurationMS != 1000 || record.TimedOutputTokens != 40 {
				t.Fatalf("timed output must be clamped to output: %+v", record)
			}
		case "untimed-1":
			if record.DurationMS != 0 || record.TimedOutputTokens != 0 {
				t.Fatalf("untimed record must stay zero: %+v", record)
			}
		}
	}
}
