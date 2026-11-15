package db_test

import (
	"token-ray/internal/db"
	"token-ray/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/a.jsonl", "mac-pro",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/b.jsonl", "linux-server",
	)

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestQueryDeviceSummariesIncludesUsageAndManualAliases(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	first := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	last := first.Add(time.Hour)
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, CachedTokens: 20, CacheCreationTokens: 5, OutputTokens: 50},
		1.25, first, "/a.jsonl", "nas.local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "m2", InputTokens: 200, OutputTokens: 75},
		2.50, last, "/b.jsonl", "nas.local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDeviceAlias("nas.local", "NAS"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDeviceAlias("manual-only", "Manual Only"); err != nil {
		t.Fatal(err)
	}

	devices, err := database.QueryDeviceSummaries()
	if err != nil {
		t.Fatal(err)
	}

	var nas *model.DeviceSummary
	var manual *model.DeviceSummary
	for i := range devices {
		if devices[i].ID == "nas.local" {
			nas = &devices[i]
		}
		if devices[i].ID == "manual-only" {
			manual = &devices[i]
		}
	}
	if nas == nil {
		t.Fatalf("expected nas.local in device summaries: %+v", devices)
	}
	if nas.DisplayName != "NAS" || nas.Events != 2 || nas.InputTokens != 300 || nas.CachedTokens != 20 || nas.CacheCreationTokens != 5 || nas.OutputTokens != 125 {
		t.Fatalf("unexpected nas summary: %+v", *nas)
	}
	if nas.TotalCost < 3.74 || nas.TotalCost > 3.76 {
		t.Fatalf("expected nas cost around 3.75, got %f", nas.TotalCost)
	}
	if !nas.FirstSeen.Equal(first) || !nas.LastSeen.Equal(last) {
		t.Fatalf("unexpected seen range: first=%s last=%s", nas.FirstSeen, nas.LastSeen)
	}
	if manual == nil || manual.DisplayName != "Manual Only" || manual.Events != 0 {
		t.Fatalf("expected manual-only alias with zero stats, got %+v", manual)
	}
}

func TestSupersedeDeviceHidesUsageAndUpdatesSourceTotals(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50, UUID: "duplicate-1"},
		1.00, now, "/a.jsonl", "probe-local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "m2", InputTokens: 200, OutputTokens: 75, UUID: "keep-1"},
		2.00, now, "/b.jsonl", "nas.local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDeviceAlias("probe-local", "Probe Local"); err != nil {
		t.Fatal(err)
	}

	changed, err := database.SupersedeDevice("probe-local")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("expected one superseded row, got %d", changed)
	}

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0] != "nas.local" {
		t.Fatalf("expected only nas.local after supersede, got %+v", devices)
	}
	stats, err := database.QuerySourceTotalsSummary("probe-local", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Fatalf("expected no active source totals for superseded device, got %+v", stats)
	}
	summaries, err := database.QueryDeviceSummaries()
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		if summary.ID == "probe-local" {
			t.Fatalf("expected superseded device alias to be hidden, got %+v", summaries)
		}
	}
}

func TestDeleteDeviceAlias(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.SetDeviceAlias("nas.local", "NAS"); err != nil {
		t.Fatal(err)
	}
	deleted, err := database.DeleteDeviceAlias("nas.local")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted alias, got %d", deleted)
	}
	aliases, err := database.GetDeviceAliases()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := aliases["nas.local"]; ok {
		t.Fatalf("expected alias to be deleted, got %+v", aliases)
	}
}
