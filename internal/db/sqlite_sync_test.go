package db_test

import (
	"token-ray/internal/db"
	"token-ray/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestSupersedeLegacyUsageBySourceFilePathsAndDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	replayableGeminiPath := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	missingGeminiPath := "/Users/c/.gemini/tmp/wiki/chats/missing.jsonl"
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 1000, OutputTokens: 300},
		1.00, now, replayableGeminiPath, "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 2000, OutputTokens: 400},
		2.00, now, replayableGeminiPath, "remote-mac",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 3000, OutputTokens: 500},
		3.00, now, missingGeminiPath, "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 4000, OutputTokens: 600},
		4.00, now, replayableGeminiPath, "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 5000, OutputTokens: 700, UUID: "gemini:new"},
		5.00, now, replayableGeminiPath, "local",
	)

	superseded, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{replayableGeminiPath}, []string{"local", "test-device"})
	if err != nil {
		t.Fatal(err)
	}
	if superseded != 1 {
		t.Fatalf("expected 1 local replayable Gemini row superseded, got %d", superseded)
	}

	geminiCost, geminiIn, _, _, geminiOut, err := database.QueryPeriodStatsAll("", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if geminiCost != 10.00 || geminiIn != 10000 || geminiOut != 1600 {
		t.Fatalf("unexpected remaining Gemini stats: cost=%f input=%d output=%d", geminiCost, geminiIn, geminiOut)
	}
	remoteGeminiCost, remoteGeminiIn, _, _, remoteGeminiOut, err := database.QueryPeriodStatsAll("remote-mac", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if remoteGeminiCost != 2.00 || remoteGeminiIn != 2000 || remoteGeminiOut != 400 {
		t.Fatalf("unexpected remote Gemini stats: cost=%f input=%d output=%d", remoteGeminiCost, remoteGeminiIn, remoteGeminiOut)
	}

	claudeCost, claudeIn, _, _, claudeOut, err := database.QueryPeriodStatsAll("", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if claudeCost != 4.00 || claudeIn != 4000 || claudeOut != 600 {
		t.Fatalf("unexpected Claude stats: cost=%f input=%d output=%d", claudeCost, claudeIn, claudeOut)
	}
}
func TestInsertUsageWithUUIDDoesNotHitLegacyDedupIndex(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ts := time.Date(2026, 4, 15, 7, 33, 41, 342_000_000, time.UTC)
	first := model.TokenUsage{
		Source:              "Claude Code",
		Model:               "claude-opus-4-6",
		InputTokens:         78442,
		CachedTokens:        77912,
		CacheCreationTokens: 529,
		OutputTokens:        204,
		UUID:                "uuid-1",
	}
	second := first
	second.UUID = "uuid-2"

	if err := database.InsertUsageWithTime(first, 1.00, ts, "/same-second.jsonl", "local"); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(second, 1.00, ts.Add(190*time.Millisecond), "/same-second.jsonl", "local"); err != nil {
		t.Fatal(err)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 2 {
		t.Fatalf("expected both UUID records to be retained, got %+v", stats)
	}
}
func TestInsertUsageWithUUIDIsScopedByDevice(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ts := time.Date(2026, 5, 7, 11, 13, 3, 0, time.UTC)
	u := model.TokenUsage{
		Source:       "Codex",
		Model:        "gpt-5.5",
		InputTokens:  1000,
		OutputTokens: 100,
		UUID:         "codex:1",
	}
	if err := database.InsertUsageWithTime(u, 1.00, ts, "/mac/logs.sqlite", "mac"); err != nil {
		t.Fatal(err)
	}
	remote := u
	remote.InputTokens = 2000
	if err := database.InsertUsageWithTime(remote, 2.00, ts, "/linux/logs.sqlite", "linux"); err != nil {
		t.Fatal(err)
	}

	cost, input, _, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 3.00 || input != 3000 || output != 200 {
		t.Fatalf("expected same UUID on different devices to retain both rows, cost=%f input=%d output=%d", cost, input, output)
	}

	update := u
	update.OutputTokens = 150
	if err := database.InsertUsageWithTime(update, 1.50, ts.Add(time.Second), "/mac/logs.sqlite", "mac"); err != nil {
		t.Fatal(err)
	}
	macCost, macInput, _, _, macOutput, err := database.QueryPeriodStatsAll("mac", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if macCost != 1.50 || macInput != 1000 || macOutput != 150 {
		t.Fatalf("expected same device UUID to upsert, cost=%f input=%d output=%d", macCost, macInput, macOutput)
	}
}
func TestSyncUsesUpdatedAtAndAppliesSupersededMarkers(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	oldLogTS := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	path := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	legacy := model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 1000, OutputTokens: 100, Timestamp: oldLogTS}
	if err := database.InsertUsageWithTime(legacy, 1.00, oldLogTS, path, "mac"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{path}, []string{"mac"}); err != nil {
		t.Fatal(err)
	}

	records, err := database.QuerySyncRecordsSince(time.Now().UTC().Add(-1 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Superseded || records[0].UpdatedAt.IsZero() {
		t.Fatalf("expected historical superseded record to sync by updated_at, got %+v", records)
	}

	peer, err := db.New(filepath.Join(t.TempDir(), "peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err := peer.InsertUsageWithTime(legacy, 1.00, oldLogTS, path, "mac"); err != nil {
		t.Fatal(err)
	}
	if err := peer.UpsertSyncRecord(records[0]); err != nil {
		t.Fatal(err)
	}
	cost, input, _, _, _, err := peer.QueryPeriodStatsAll("mac", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 0 || input != 0 {
		t.Fatalf("expected sync superseded marker to hide peer legacy row, cost=%f input=%d", cost, input)
	}
}
func TestQuerySyncRecordsPageUsesCursor(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, UUID: "sync-page-1"},
		1.00, now, "/one.jsonl", "mac",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, UUID: "sync-page-2"},
		2.00, now.Add(time.Second), "/two.jsonl", "mac",
	); err != nil {
		t.Fatal(err)
	}

	first, err := database.QuerySyncRecordsPage(time.Time{}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || !first.HasMore || first.NextUpdatedAt.IsZero() || first.NextAfterID == 0 {
		t.Fatalf("unexpected first page: %+v", first)
	}
	second, err := database.QuerySyncRecordsPage(first.NextUpdatedAt, first.NextAfterID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].UUID == first.Records[0].UUID {
		t.Fatalf("expected second page to advance cursor, first=%+v second=%+v", first, second)
	}
}
func TestQuerySyncRecordsPageCanFilterDevice(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, UUID: "local-old"},
		1.00, now, "/local.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, UUID: "remote-new"},
		2.00, now.Add(time.Second), "/remote.jsonl", "remote",
	); err != nil {
		t.Fatal(err)
	}

	page, err := database.QuerySyncRecordsPageForDevice(time.Time{}, 0, 1, "remote")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].DeviceID != "remote" || page.Records[0].UUID != "remote-new" {
		t.Fatalf("expected remote-only sync page, got %+v", page)
	}
	if page.HasMore {
		t.Fatalf("expected no more remote records, got %+v", page)
	}
}
func TestUpsertSyncRecordPreservesRemoteUpdatedAt(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	logTS := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	remoteUpdatedAt := time.Date(2026, 5, 2, 13, 0, 0, 0, time.UTC)
	record := model.SyncRecord{
		TokenUsage: model.TokenUsage{
			Source:       "Claude Code",
			Model:        "claude-opus-4-7",
			InputTokens:  100,
			OutputTokens: 10,
			Timestamp:    logTS,
			UUID:         "preserve-updated-at",
		},
		CostUSD:   1.00,
		FilePath:  "/remote.jsonl",
		DeviceID:  "remote",
		UpdatedAt: remoteUpdatedAt,
	}
	if err := database.UpsertSyncRecord(record); err != nil {
		t.Fatal(err)
	}

	page, err := database.QuerySyncRecordsPage(time.Time{}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 {
		t.Fatalf("expected one sync record, got %+v", page)
	}
	if !page.Records[0].UpdatedAt.Equal(remoteUpdatedAt) {
		t.Fatalf("expected remote updated_at %s preserved, got %s", remoteUpdatedAt, page.Records[0].UpdatedAt)
	}
}

func TestUpsertSyncRecordDoesNotReviveNewerSupersededUUIDRow(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	logTS := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, Timestamp: logTS, UUID: "duplicate-device-row"},
		1.00, logTS, "/remote.jsonl", "probe-local",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeDevice("probe-local"); err != nil {
		t.Fatal(err)
	}

	olderActive := model.SyncRecord{
		TokenUsage: model.TokenUsage{
			Source:       "Claude Code",
			Model:        "claude-opus-4-7",
			InputTokens:  100,
			OutputTokens: 10,
			Timestamp:    logTS,
			UUID:         "duplicate-device-row",
		},
		CostUSD:   1.00,
		FilePath:  "/remote.jsonl",
		DeviceID:  "probe-local",
		UpdatedAt: logTS.Add(time.Hour),
	}
	if err := database.UpsertSyncRecord(olderActive); err != nil {
		t.Fatal(err)
	}

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range devices {
		if device == "probe-local" {
			t.Fatalf("older active sync record revived superseded device: %+v", devices)
		}
	}
}

func TestUpsertSyncRecordDoesNotReviveNewerSupersededLegacyRow(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	logTS := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	usage := model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, Timestamp: logTS}
	if err := database.InsertUsageWithTime(usage, 1.00, logTS, "/legacy.jsonl", "probe-local"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeDevice("probe-local"); err != nil {
		t.Fatal(err)
	}

	if err := database.UpsertSyncRecord(model.SyncRecord{
		TokenUsage: usage,
		CostUSD:    1.00,
		FilePath:   "/legacy.jsonl",
		DeviceID:   "probe-local",
		UpdatedAt:  logTS.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range devices {
		if device == "probe-local" {
			t.Fatalf("older legacy active sync record revived superseded device: %+v", devices)
		}
	}
}
