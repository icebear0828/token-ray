package codexusage_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func TestScanImportsCodexTelemetryEvents(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	createCodexState(t, statePath)
	createCodexLogs(t, logsPath)

	insertThread(t, statePath, "thread-1", "gpt-5.5", "/Users/c/token")
	insertLog(t, logsPath, 9, `session_loop{thread_id=thread-1}: event.name="codex.tool_result" event.kind=response.completed input_token_count=999 output_token_count=999 cached_token_count=999 event.timestamp=2026-05-07T11:13:02.000Z conversation.id=thread-1 model=gpt-5.5`)
	insertLog(t, logsPath, 10, `event.name="codex.sse_event" event.kind=response.completed duration_ms=178 event.timestamp=2026-05-07T11:13:03.315Z conversation.id=thread-1 model=gpt-5.5`)
	insertLog(t, logsPath, 11, `event.name="codex.sse_event" event.kind=response.completed input_token_count=115541 output_token_count=713 cached_token_count=49024 reasoning_token_count=261 tool_token_count=116254 event.timestamp=2026-05-07T11:13:03.316Z conversation.id=thread-1 model=gpt-5.5 slug=gpt-5.5`)
	insertLog(t, logsPath, 12, `event.name="codex.sse_event" event.kind=response.completed input_token_count=999 output_token_count=1 cached_token_count=0 event.timestamp=2026-05-07T11:13:05.000Z conversation.id=missing-thread model=gpt-5.5`)

	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported telemetry event, got %d", count)
	}

	cost, input, cached, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 115541 || cached != 49024 || output != 713 {
		t.Errorf("unexpected token totals: input=%d cached=%d output=%d", input, cached, output)
	}
	if cost <= 0 {
		t.Errorf("expected Codex cost to be non-zero for priced model, got %f", cost)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "token" {
		t.Fatalf("expected Codex project token, got %+v", projects)
	}

	insertThread(t, statePath, "missing-thread", "gpt-5.5", "/Users/c/recovered")

	secondCount, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if secondCount != 1 {
		t.Fatalf("expected second scan to import recovered telemetry event, got %d", secondCount)
	}

	cost, input, cached, _, output, err = database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 116540 || cached != 49024 || output != 714 {
		t.Errorf("unexpected recovered token totals: input=%d cached=%d output=%d", input, cached, output)
	}
	if cost <= 0 {
		t.Errorf("expected recovered Codex cost to remain non-zero, got %f", cost)
	}

	thirdCount, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if thirdCount != 0 {
		t.Fatalf("expected third scan to be incremental, got %d", thirdCount)
	}
}

func TestScanImportsPrefixedCodexSSECompletionEvents(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	createCodexState(t, statePath)
	createCodexLogs(t, logsPath)

	insertThread(t, statePath, "thread-1", "gpt-5.5", "/Users/c/token")
	insertLog(t, logsPath, 21, `session_loop{thread_id=thread-1}:turn{model=gpt-5.5}: event.name="codex.sse_event" event.kind=response.completed input_token_count=321 output_token_count=45 cached_token_count=123 reasoning_token_count=9 event.timestamp=2026-05-07T11:14:03.316Z conversation.id=thread-1 model=gpt-5.5 slug=gpt-5.5`)

	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected prefixed Codex SSE completion to import, got %d", count)
	}

	_, input, cached, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 321 || cached != 123 || output != 45 {
		t.Fatalf("unexpected prefixed event totals: input=%d cached=%d output=%d", input, cached, output)
	}
}

func TestScanImportsCodexSessionTokenTotals(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	sessionsDir := filepath.Join(dir, "sessions")
	createCodexState(t, statePath)
	createCodexLogs(t, logsPath)

	insertThread(t, statePath, "session-1", "gpt-5.5", "/Users/c/token")
	sessionPath := writeCodexSession(t, sessionsDir, "session-1", "/Users/c/token",
		`{"timestamp":"2026-05-07T11:13:03.316Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":10,"reasoning_output_tokens":3,"total_tokens":1010},"last_token_usage":{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":10,"reasoning_output_tokens":3,"total_tokens":1010},"model_context_window":258400}}}`,
		`{"timestamp":"2026-05-07T11:14:03.316Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":3000,"cached_input_tokens":2500,"output_tokens":50,"reasoning_output_tokens":7,"total_tokens":3050},"last_token_usage":{"input_tokens":2000,"cached_input_tokens":1700,"output_tokens":40,"reasoning_output_tokens":4,"total_tokens":2040},"model_context_window":258400}}}`,
	)

	s := codexusage.NewWithSessionPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath, sessionsDir)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported session total, got %d", count)
	}

	_, input, cached, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 3000 || cached != 2500 || output != 50 {
		t.Fatalf("unexpected session totals: input=%d cached=%d output=%d", input, cached, output)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "token" {
		t.Fatalf("expected Codex session project token, got %+v", projects)
	}

	secondCount, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if secondCount != 0 {
		t.Fatalf("expected unchanged session scan to import 0 rows, got %d", secondCount)
	}

	appendCodexSessionLine(t, sessionPath, `{"timestamp":"2026-05-07T11:15:03.316Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":4000,"cached_input_tokens":3000,"output_tokens":90,"reasoning_output_tokens":11,"total_tokens":4090},"last_token_usage":{"input_tokens":1000,"cached_input_tokens":500,"output_tokens":40,"reasoning_output_tokens":4,"total_tokens":1040},"model_context_window":258400}}}`)

	thirdCount, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if thirdCount != 1 {
		t.Fatalf("expected changed session scan to update 1 row, got %d", thirdCount)
	}

	_, input, cached, _, output, err = database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 4000 || cached != 3000 || output != 90 {
		t.Fatalf("unexpected updated session totals: input=%d cached=%d output=%d", input, cached, output)
	}
}

func TestScanSessionTotalsSupersedeCodexLogTelemetryRows(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	sessionsDir := filepath.Join(dir, "sessions")
	createCodexState(t, statePath)
	createCodexLogs(t, logsPath)

	insertThread(t, statePath, "session-1", "gpt-5.5", "/Users/c/token")
	insertLog(t, logsPath, 11, `event.name="codex.sse_event" event.kind=response.completed input_token_count=100 output_token_count=10 cached_token_count=80 event.timestamp=2026-05-07T11:13:03.316Z conversation.id=session-1 model=gpt-5.5`)

	logOnlyScanner := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	if count, err := logOnlyScanner.Scan(); err != nil || count != 1 {
		t.Fatalf("expected log-only scan to import 1 row, count=%d err=%v", count, err)
	}

	writeCodexSession(t, sessionsDir, "session-1", "/Users/c/token",
		`{"timestamp":"2026-05-07T11:14:03.316Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":3000,"cached_input_tokens":2500,"output_tokens":50,"reasoning_output_tokens":7,"total_tokens":3050},"last_token_usage":{"input_tokens":3000,"cached_input_tokens":2500,"output_tokens":50,"reasoning_output_tokens":7,"total_tokens":3050},"model_context_window":258400}}}`,
	)

	sessionScanner := codexusage.NewWithSessionPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath, sessionsDir)
	if count, err := sessionScanner.Scan(); err != nil || count != 1 {
		t.Fatalf("expected session scan to import 1 row, count=%d err=%v", count, err)
	}

	_, input, cached, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 3000 || cached != 2500 || output != 50 {
		t.Fatalf("expected session totals to supersede log telemetry, input=%d cached=%d output=%d", input, cached, output)
	}
}

func TestScanIgnoresToolResultTextThatMentionsCodexSSECompletion(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	createCodexState(t, statePath)
	createCodexLogs(t, logsPath)

	insertThread(t, statePath, "thread-1", "gpt-5.5", "/Users/c/token")
	insertLog(t, logsPath, 31, `session_loop{thread_id=thread-1}:turn{model=gpt-5.5}: event.name="codex.tool_result" tool_name=exec_command arguments={"cmd":"sqlite3 logs_2.sqlite \"select * from logs where event.name=\"codex.sse_event\" and event.kind=response.completed and input_token_count=777\""} duration_ms=10 success=true output=event.name="codex.sse_event" event.kind=response.completed input_token_count=777 output_token_count=88 cached_token_count=66 event.timestamp=2026-05-07T11:15:03.316Z conversation.id=thread-1 model=gpt-5.5 event.timestamp=2026-05-07T11:15:04.000Z conversation.id=thread-1 model=gpt-5.5`)

	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected tool result text to be ignored, got %d imports", count)
	}

	_, input, cached, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 0 || cached != 0 || output != 0 {
		t.Fatalf("expected no Codex totals from tool result text, input=%d cached=%d output=%d", input, cached, output)
	}
}

func TestScanSkipsMissingOptionalCodexFiles(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),filepath.Join(dir, "missing-state.sqlite"), filepath.Join(dir, "missing-logs.sqlite"))
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected missing optional Codex files to scan 0 rows, got %d", count)
	}
}

func TestScanSkipsEmptyOptionalCodexDatabases(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	createEmptySQLite(t, statePath)
	createEmptySQLite(t, logsPath)

	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected empty optional Codex DBs to scan 0 rows, got %d", count)
	}
}

func TestScanSkipsMalformedOptionalCodexDatabases(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	if err := os.WriteFile(statePath, []byte("not sqlite"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logsPath, []byte("not sqlite"), 0644); err != nil {
		t.Fatal(err)
	}

	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected malformed optional Codex DBs to scan 0 rows, got %d", count)
	}
}

func TestScanSkipsIncompatibleOptionalCodexSchema(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	createSQLiteWithSchema(t, statePath, "CREATE TABLE threads (unexpected TEXT)")
	createSQLiteWithSchema(t, logsPath, "CREATE TABLE logs (unexpected TEXT)")

	s := codexusage.NewWithPaths(testutil.NewTestSink(t, database, calc, "local"),statePath, logsPath)
	count, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected incompatible optional Codex schema to scan 0 rows, got %d", count)
	}
}

func createCodexState(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(`
		CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			model TEXT,
			cwd TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func createEmptySQLite(t *testing.T, path string) {
	t.Helper()
	createSQLiteWithSchema(t, path, "")
}

func createSQLiteWithSchema(t *testing.T, path string, schema string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if schema == "" {
		if err := conn.Ping(); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := conn.Exec(schema); err != nil {
		t.Fatal(err)
	}
}

func createCodexLogs(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(`
		CREATE TABLE logs (
			id INTEGER PRIMARY KEY,
			ts INTEGER NOT NULL,
			ts_nanos INTEGER NOT NULL,
			target TEXT NOT NULL,
			feedback_log_body TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func insertThread(t *testing.T, path string, id string, model string, cwd string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec("INSERT INTO threads (id, model, cwd) VALUES (?, ?, ?)", id, model, cwd)
	if err != nil {
		t.Fatal(err)
	}
}

func insertLog(t *testing.T, path string, id int64, body string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(
		"INSERT INTO logs (id, ts, ts_nanos, target, feedback_log_body) VALUES (?, ?, ?, ?, ?)",
		id, time.Date(2026, 5, 7, 11, 13, int(id), 0, time.UTC).Unix(), int64(0), "codex_otel.log_only", body,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func writeCodexSession(t *testing.T, sessionsDir string, id string, cwd string, tokenLines ...string) string {
	t.Helper()
	dir := filepath.Join(sessionsDir, "2026", "05", "07")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-05-07T11-13-03-"+id+".jsonl")
	lines := []byte(`{"timestamp":"2026-05-07T11:13:00.000Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `","model_provider":"openai","source":"cli"}}` + "\n")
	for _, line := range tokenLines {
		lines = append(lines, []byte(line+"\n")...)
	}
	if err := os.WriteFile(path, lines, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendCodexSessionLine(t *testing.T, path string, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}
