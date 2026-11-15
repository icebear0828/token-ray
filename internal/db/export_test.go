package db_test

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"token-ray/internal/db"
	"token-ray/internal/model"
)

func TestExportCSV(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200},
		1.50, now, "/a.jsonl", "mac-pro",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 800, CachedTokens: 0, OutputTokens: 100},
		0.60, now.Add(time.Minute), "/b.jsonl", "mac-pro",
	)

	var buf bytes.Buffer
	count, err := database.ExportCSV(&buf, "")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows exported, got %d", count)
	}

	output := buf.String()
	// Should have header + 2 data rows
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	// header + 2 data + trailing newline
	nonEmpty := 0
	for _, l := range lines {
		if len(l) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty != 3 { // header + 2 rows
		t.Errorf("expected 3 non-empty lines (header+2), got %d\n%s", nonEmpty, output)
	}

	// Header should contain expected columns
	header := string(lines[0])
	for _, col := range []string{"log_timestamp", "source", "model", "input_tokens", "cached_tokens", "cache_creation_tokens", "output_tokens", "cost_usd", "file_path", "device_id"} {
		if !bytes.Contains([]byte(header), []byte(col)) {
			t.Errorf("header missing column %q: %s", col, header)
		}
	}
}

func TestExportCSV_DeviceFilter(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/a.jsonl", "mac",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 200, OutputTokens: 100},
		2.00, now.Add(time.Minute), "/b.jsonl", "linux",
	)

	var buf bytes.Buffer
	count, err := database.ExportCSV(&buf, "mac")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 row for device=mac, got %d", count)
	}
}

func TestExportCSVSkipsSupersededRows(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	path := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 100, OutputTokens: 50},
		1.00, now, path, "mac",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 200, OutputTokens: 100, UUID: "gemini:active"},
		2.00, now.Add(time.Second), path, "mac",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{path}, []string{"mac"}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	count, err := database.ExportCSV(&buf, "")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one active exported row, got %d\n%s", count, buf.String())
	}
	if bytes.Contains(buf.Bytes(), []byte(",100,0,0,50,")) {
		t.Fatalf("export included superseded legacy row:\n%s", buf.String())
	}
}

func TestImportCSV(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	csv := `log_timestamp,source,model,input_tokens,cached_tokens,cache_creation_tokens,output_tokens,cost_usd,file_path,device_id
2026-04-20T10:00:00Z,Claude Code,claude-opus-4-7,1000,500,200,200,1.50,/a.jsonl,remote-mac
2026-04-20T10:01:00Z,Gemini CLI,gemini-2.5-pro,800,0,0,100,0.60,/b.jsonl,remote-mac
`
	reader := bytes.NewBufferString(csv)
	imported, skipped, err := database.ImportCSV(reader)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify data in DB
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("remote-mac", "")
	if cost < 2.09 || cost > 2.11 {
		t.Errorf("expected ~2.10 cost, got %f", cost)
	}
}

func TestImportCSV_Dedup(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	csv := `log_timestamp,source,model,input_tokens,cached_tokens,cache_creation_tokens,output_tokens,cost_usd,file_path,device_id
2026-04-20T10:00:00Z,Claude Code,claude-opus-4-7,1000,500,200,200,1.50,/a.jsonl,mac
`
	// Import once
	reader := bytes.NewBufferString(csv)
	database.ImportCSV(reader)

	// Import same data again — should be skipped
	reader = bytes.NewBufferString(csv)
	imported, skipped, err := database.ImportCSV(reader)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 0 {
		t.Errorf("expected 0 imported (dedup), got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped (dedup), got %d", skipped)
	}

	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 1.49 || cost > 1.51 {
		t.Errorf("expected ~1.50 (no dup), got %f", cost)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	// Create source DB with data
	srcDB, _ := db.New(filepath.Join(t.TempDir(), "src.db"))
	defer srcDB.Close()

	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	srcDB.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200},
		1.50, now, "/a.jsonl", "device-A",
	)
	srcDB.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 800, CachedTokens: 0, OutputTokens: 100},
		0.60, now.Add(time.Minute), "/b.jsonl", "device-A",
	)

	// Export
	var buf bytes.Buffer
	srcDB.ExportCSV(&buf, "")

	// Import into a fresh DB
	dstDB, _ := db.New(filepath.Join(t.TempDir(), "dst.db"))
	defer dstDB.Close()

	imported, _, err := dstDB.ImportCSV(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}

	// Verify totals match
	srcCost, _, _, _, _, _ := srcDB.QueryPeriodStatsAll("", "")
	dstCost, _, _, _, _, _ := dstDB.QueryPeriodStatsAll("", "")
	if srcCost < dstCost-0.01 || srcCost > dstCost+0.01 {
		t.Errorf("round-trip cost mismatch: src=%f dst=%f", srcCost, dstCost)
	}
}
