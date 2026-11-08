package scanner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/scanner"
	"ai-flight-dashboard/internal/testutil"
)

func TestScanAll_FullScan(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	// Create fake Claude log directory structure
	logDir := t.TempDir()
	projectDir := filepath.Join(logDir, "projects", "-Users-c-test")
	os.MkdirAll(projectDir, 0755)

	ts := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	logFile := filepath.Join(projectDir, "session-abc.jsonl")
	content := fmt.Sprintf(
		`{"parentUuid":"x","timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1000,"cache_read_input_tokens":5000,"output_tokens":200}}}`,
		ts,
	) + "\n"
	// Write 3 usage lines
	os.WriteFile(logFile, []byte(content+content+content), 0644)

	s := scanner.New(testutil.NewTestSink(t, database, calc, "test-device"))
	count, err := s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 records scanned, got %d", count)
	}

	// Verify DB has data
	total, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if total < 0.01 {
		t.Errorf("expected non-zero cost, got %f", total)
	}
}

func TestScanAll_IncrementalScan(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	logFile := filepath.Join(logDir, "session.jsonl")
	line := fmt.Sprintf(
		`{"timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":500,"cache_read_input_tokens":0,"output_tokens":100}}}`,
		ts,
	) + "\n"

	// First scan: 2 lines
	os.WriteFile(logFile, []byte(line+line), 0644)
	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count1, _ := s.ScanAll([]string{logDir})
	if count1 != 2 {
		t.Errorf("first scan: expected 2, got %d", count1)
	}

	// Second scan without changes: should be 0
	count2, _ := s.ScanAll([]string{logDir})
	if count2 != 0 {
		t.Errorf("second scan: expected 0 (no new data), got %d", count2)
	}

	// Append 1 more line
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(line)
	f.Close()

	// Third scan: only the new line
	count3, _ := s.ScanAll([]string{logDir})
	if count3 != 1 {
		t.Errorf("third scan: expected 1 (incremental), got %d", count3)
	}
}

func TestScanKnownFilesUsesPreloadedOffsets(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "session.jsonl")
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf(
		`{"timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":500,"cache_read_input_tokens":0,"output_tokens":100}}}`,
		ts,
	) + "\n"
	if err := os.WriteFile(logFile, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	if count, err := s.ScanAll([]string{logDir}); err != nil || count != 1 {
		t.Fatalf("full scan: count=%d err=%v", count, err)
	}

	// Unchanged file: the known-file scan must read the offset from the
	// preloaded map and skip the file.
	if count, err := s.ScanKnownFiles([]string{logDir}); err != nil || count != 0 {
		t.Fatalf("unchanged known-file scan: count=%d err=%v", count, err)
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if count, err := s.ScanKnownFiles([]string{logDir}); err != nil || count != 1 {
		t.Fatalf("appended known-file scan: count=%d err=%v", count, err)
	}
}

func TestScanAllStrictReturnsFileErrors(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "session.jsonl")
	line := `{"timestamp":"2026-05-07T11:13:03Z","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":500,"cache_read_input_tokens":0,"output_tokens":100}}}` + "\n"
	if err := os.WriteFile(logFile, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	if _, err := s.ScanAllStrict([]string{logDir}); err == nil {
		t.Fatal("expected strict scan to return the closed database error")
	}
	if count, err := s.ScanAll([]string{logDir}); err != nil || count != 0 {
		t.Fatalf("non-strict scan should skip file errors, count=%d err=%v", count, err)
	}
}

func TestScanAll_GeminiFormat(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	chatsDir := filepath.Join(logDir, "chats")
	os.MkdirAll(chatsDir, 0755)

	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	logFile := filepath.Join(chatsDir, "session-2026.jsonl")
	content := fmt.Sprintf(
		`{"id":"abc","timestamp":"%s","type":"gemini","content":"Hello","tokens":{"input":8000,"output":5,"cached":0},"model":"gemini-2.5-pro"}`,
		ts,
	) + "\n"
	os.WriteFile(logFile, []byte(content), 0644)

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count, _ := s.ScanAll([]string{logDir})
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	total, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if total < 0.01 {
		t.Errorf("expected non-zero Gemini cost, got %f", total)
	}
}

func TestScanAll_GeminiWithoutIDGetsStableUUID(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	chatsDir := filepath.Join(logDir, ".gemini", "tmp", "wiki", "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(chatsDir, "session.jsonl")
	lines := `{"timestamp":"2026-05-07T11:13:03.316Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-3.1-pro-preview"}` + "\n" +
		`{"timestamp":"2026-05-07T11:13:04.316Z","type":"gemini","tokens":{"input":2000,"output":75,"cached":300},"model":"gemini-3.1-pro-preview"}` + "\n"
	if err := os.WriteFile(logFile, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count, err := s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected first scan to import 2 rows, got %d", count)
	}
	if err := database.SetOffset(logFile, 0); err != nil {
		t.Fatal(err)
	}
	count, err = s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected rescan to upsert 2 rows, got %d", count)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 2 || stats[0].InputTokens != 3000 || stats[0].OutputTokens != 125 {
		t.Fatalf("expected stable UUID rescan to avoid duplicates, got %+v", stats)
	}
}

func TestScanAllStrictProcessesFinalLineWithoutTrailingNewline(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "session.jsonl")
	line := `{"timestamp":"2026-05-07T11:13:03.316Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-3.1-pro-preview"}`
	if err := os.WriteFile(logFile, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count, err := s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected non-strict scan to wait for newline, got %d", count)
	}
	count, err = s.ScanAllStrict([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected strict scan to process final line, got %d", count)
	}
}

func TestScanAllStrictDoesNotAdvanceInvalidPartialEOF(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "session.jsonl")
	complete := `{"timestamp":"2026-05-07T11:13:03.316Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-3.1-pro-preview"}` + "\n"
	partial := `{"timestamp":"2026-05-07T11:13:04.316Z","type":"gemini"`
	if err := os.WriteFile(logFile, []byte(complete+partial), 0644); err != nil {
		t.Fatal(err)
	}

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count, err := s.ScanAllStrict([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected strict scan to import only complete row, got %d", count)
	}
	offset, err := database.GetOffset(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if offset != int64(len(complete)) {
		t.Fatalf("expected offset to stop before invalid partial EOF, got %d want %d", offset, len(complete))
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`,"tokens":{"input":2000,"output":75,"cached":300},"model":"gemini-3.1-pro-preview"}` + "\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	count, err = s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected completed partial row to import later, got %d", count)
	}
	stats, err := database.QueryStatsSince(time.Time{}, "", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 2 || stats[0].InputTokens != 3000 {
		t.Fatalf("expected both rows after partial completion, got %+v", stats)
	}
}

func TestScanAll_ProjectPrefersClaudeCWDOverFolderFallback(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	projectDir := filepath.Join(logDir, "projects", "-Users-john-doe-my-hyphen-app")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	logFile := filepath.Join(projectDir, "session-abc.jsonl")
	metadata := `{"type":"system","cwd":"/Users/john-doe/my-hyphen-app"}` + "\n"
	content := fmt.Sprintf(
		`{"uuid":"abc","timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1000,"cache_read_input_tokens":0,"output_tokens":200}}}`,
		ts,
	) + "\n"
	if err := os.WriteFile(logFile, []byte(metadata+content), 0644); err != nil {
		t.Fatal(err)
	}

	s := scanner.New(testutil.NewTestSink(t, database, calc, "test-device"))
	count, err := s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 record scanned, got %d", count)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "my-hyphen-app" {
		t.Fatalf("expected cwd-derived project my-hyphen-app, got %+v", projects)
	}
}

func TestScanAll_SkipsNonUsageLines(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "session.jsonl")
	// Mix of usage and non-usage lines
	content := `{"type":"system","content":"init"}
{"parentUuid":"x","timestamp":"2026-04-20T10:00:00Z","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":100,"cache_read_input_tokens":0,"output_tokens":50}}}
{"type":"tool_result","content":"ok"}
`
	os.WriteFile(logFile, []byte(content), 0644)

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count, _ := s.ScanAll([]string{logDir})
	if count != 1 {
		t.Errorf("expected 1 usage line only, got %d", count)
	}
}

func TestScanAll_FileTruncation(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	logDir := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	logFile := filepath.Join(logDir, "session.jsonl")

	// Write 3 lines initially
	line := fmt.Sprintf(
		`{"timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":500,"cache_read_input_tokens":0,"output_tokens":100}}}`,
		ts,
	) + "\n"
	os.WriteFile(logFile, []byte(line+line+line), 0644)

	s := scanner.New(testutil.NewTestSink(t, database, calc, "local"))
	count1, _ := s.ScanAll([]string{logDir})
	if count1 != 3 {
		t.Errorf("first scan: expected 3, got %d", count1)
	}

	// Truncate and write 1 shorter line (simulates log rotation)
	os.WriteFile(logFile, []byte(line), 0644)

	count2, _ := s.ScanAll([]string{logDir})
	if count2 != 1 {
		t.Errorf("after truncation: expected 1 (re-read from start), got %d", count2)
	}
}
