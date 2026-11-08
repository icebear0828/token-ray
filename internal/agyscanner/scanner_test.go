package agyscanner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/watcher"
)

func newTestSink(t *testing.T, database *db.DB, calc *calculator.Calculator, deviceID string) (source.Sink, chan watcher.TokenUsage) {
	t.Helper()
	ch := make(chan watcher.TokenUsage, 100)
	sink := source.NewDBSink(database, calc, deviceID, func(u model.TokenUsage) {
		select {
		case ch <- u:
		default:
		}
	})
	return sink, ch
}

func fakeTrajectoryJSON(cascadeID string, steps []GeneratorMetadata) string {
	resp := TrajectoryResponse{
		Trajectory: Trajectory{
			CascadeID:         cascadeID,
			GeneratorMetadata: steps,
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func fakeSummariesJSON(ids ...string) string {
	summaries := make(map[string]TrajectorySummary)
	for _, id := range ids {
		summaries[id] = TrajectorySummary{StepCount: 10, Status: "CASCADE_RUN_STATUS_IDLE"}
	}
	resp := TrajectoriesResponse{Summaries: summaries}
	b, _ := json.Marshal(resp)
	return string(b)
}

func makeStep(input, output, cached int, model, createdAt string) GeneratorMetadata {
	return GeneratorMetadata{
		ChatModel: ChatModel{
			Usage: ChatModelUsage{
				InputTokens:     strconv.Itoa(input),
				OutputTokens:    strconv.Itoa(output),
				CacheReadTokens: strconv.Itoa(cached),
			},
			ResponseModel:    model,
			ModelDisplayName: "Test Model",
			ChatStartMetadata: ChatStartMetadata{
				CreatedAt: createdAt,
			},
		},
	}
}

func TestClientParsing(t *testing.T) {
	cascadeID := "test-cascade-001"
	steps := []GeneratorMetadata{
		makeStep(1000, 200, 500, "gemini-3-flash-a", "2026-05-26T10:00:00Z"),
		makeStep(2000, 300, 0, "gemini-3-flash-a", "2026-05-26T10:01:00Z"),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			w.Write([]byte(fakeSummariesJSON(cascadeID)))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			w.Write([]byte(fakeTrajectoryJSON(cascadeID, steps)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	port := portFromURL(t, srv.URL)

	summaries, err := getAllTrajectories(port)
	if err != nil {
		t.Fatalf("getAllTrajectories: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if _, ok := summaries[cascadeID]; !ok {
		t.Fatalf("missing cascade %s", cascadeID)
	}

	resp, err := getTrajectory(port, cascadeID)
	if err != nil {
		t.Fatalf("getTrajectory: %v", err)
	}
	if len(resp.Trajectory.GeneratorMetadata) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(resp.Trajectory.GeneratorMetadata))
	}
	u := resp.Trajectory.GeneratorMetadata[0].ChatModel.Usage
	if u.InputInt() != 1000 || u.OutputInt() != 200 || u.CacheReadInt() != 500 {
		t.Errorf("tokens mismatch: in=%d out=%d cache=%d", u.InputInt(), u.OutputInt(), u.CacheReadInt())
	}
}

func TestScanIncrementalOffset(t *testing.T) {
	cascadeID := "offset-test-001"

	step1 := makeStep(1000, 100, 0, "gemini-3-flash-a", "2026-05-26T10:00:00Z")
	step2 := makeStep(2000, 200, 500, "gemini-3-flash-a", "2026-05-26T10:01:00Z")

	currentSteps := []GeneratorMetadata{step1}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			w.Write([]byte(fakeSummariesJSON(cascadeID)))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			w.Write([]byte(fakeTrajectoryJSON(cascadeID, currentSteps)))
		}
	}))
	defer srv.Close()

	port := portFromURL(t, srv.URL)
	database, calc := testutil.NewTestDBAndCalc(t)
	sink, ch := newTestSink(t, database, calc, "test-device")
	s := &Scanner{
		sink:     sink,
		LogDir:   t.TempDir(),
		HistPath: filepath.Join(t.TempDir(), "history.jsonl"),
	}

	n, err := s.ScanWithPort(port)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("first scan: expected 1 new record, got %d", n)
	}

	// Drain first scan result
	u := <-ch
	if u.InputTokens != 1000 {
		t.Errorf("first record input=%d, want 1000", u.InputTokens)
	}

	// Second scan with same data — should produce 0 new records
	n, err = s.ScanWithPort(port)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if n != 0 {
		t.Fatalf("second scan: expected 0 new records (offset tracking), got %d", n)
	}

	// Add a second step
	currentSteps = []GeneratorMetadata{step1, step2}
	n, err = s.ScanWithPort(port)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("third scan: expected 1 new record (incremental), got %d", n)
	}

	u = <-ch
	if u.InputTokens != 2500 || u.CachedTokens != 500 {
		t.Errorf("third record: input=%d cached=%d, want 2500/500", u.InputTokens, u.CachedTokens)
	}
}

func TestWorkspaceMapping(t *testing.T) {
	histDir := t.TempDir()
	histPath := filepath.Join(histDir, "history.jsonl")
	lines := []string{
		`{"conversationId":"conv-001","workspace":"/Users/c/myproject","display":"hello","timestamp":1}`,
		`{"conversationId":"conv-002","workspace":"/Users/c/other","display":"world","timestamp":2}`,
	}
	os.WriteFile(histPath, []byte(strings.Join(lines, "\n")), 0644)

	s := &Scanner{HistPath: histPath}
	ws := s.loadWorkspaces()
	if ws["conv-001"] != "/Users/c/myproject" {
		t.Errorf("conv-001 workspace=%q, want /Users/c/myproject", ws["conv-001"])
	}
	if ws["conv-002"] != "/Users/c/other" {
		t.Errorf("conv-002 workspace=%q, want /Users/c/other", ws["conv-002"])
	}
}

func TestDiscoveryParseLog(t *testing.T) {
	logDir := t.TempDir()
	logContent := `I0526 08:17:44.316284 28072 server.go:487] Language server listening on random port at 58848 for HTTPS (gRPC)
I0526 08:17:44.316464 28072 server.go:494] Language server listening on random port at 58849 for HTTP
I0526 13:38:34.290504 63320 server.go:487] Language server listening on random port at 51354 for HTTPS (gRPC)
I0526 13:38:34.290677 63320 server.go:494] Language server listening on random port at 51355 for HTTP
`
	os.WriteFile(filepath.Join(logDir, "cli-20260526_081743.log"), []byte(logContent), 0644)

	ports := parseLogPorts(filepath.Join(logDir, "cli-20260526_081743.log"))
	if len(ports) != 2 {
		t.Fatalf("expected 2 HTTP ports, got %d", len(ports))
	}
	if ports[0].Port != 58849 || ports[0].PID != 28072 {
		t.Errorf("first port: %+v, want {PID:28072 Port:58849}", ports[0])
	}
	if ports[1].Port != 51355 || ports[1].PID != 63320 {
		t.Errorf("second port: %+v, want {PID:63320 Port:51355}", ports[1])
	}
}

func TestParseTimestamp(t *testing.T) {
	ts := parseTimestamp("2026-05-26T15:18:09.130264Z")
	if ts.Year() != 2026 || ts.Month() != 5 || ts.Day() != 26 {
		t.Errorf("unexpected date: %v", ts)
	}
	if ts.Hour() != 15 || ts.Minute() != 18 {
		t.Errorf("unexpected time: %v", ts)
	}
}

func TestNormalizeResponseModel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"gemini-3-flash-a", "gemini-3-flash"},
		{"gemini-3.1-pro-high", "gemini-3.1-pro"},
		{"gemini-3-flash-c", "gemini-3-flash"},
		{"claude-opus-4-6-thinking", "claude-opus-4-6"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"gemini-3.5-flash-extra-low", "gemini-3.5-flash"},
		{"gemini-default", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeResponseModel(c.in)
		if got != c.want {
			t.Errorf("normalizeResponseModel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func portFromURL(t *testing.T, url string) int {
	t.Helper()
	parts := strings.Split(url, ":")
	port, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		t.Fatalf("parse port from %q: %v", url, err)
	}
	return port
}

func TestNormalizeDisplayName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Gemini 3.5 Flash (Low)", "gemini-3.5-flash"},
		{"Gemini 3.5 Flash (Medium)", "gemini-3.5-flash"},
		{"Gemini 3.1 Pro (High)", "gemini-3.1-pro"},
		{"Claude Sonnet 4.6 (Thinking)", "claude-sonnet-4-6"},
		{"Claude Opus 4.6 (Thinking)", "claude-opus-4-6"},
		{"GPT-OSS 120B (Medium)", "gpt-oss-120b"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeDisplayName(c.in)
		if got != c.want {
			t.Errorf("normalizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClientCsrfHeader(t *testing.T) {
	const expectedToken = "test-csrf-token-xyz"
	var receivedToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("x-codeium-csrf-token")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fakeSummariesJSON("conv-csrf")))
	}))
	defer srv.Close()

	t.Setenv("ANTIGRAVITY_CSRF_TOKEN", expectedToken)
	port := portFromURL(t, srv.URL)

	_, err := getAllTrajectories(port)
	if err != nil {
		t.Fatalf("getAllTrajectories: %v", err)
	}
	if receivedToken != expectedToken {
		t.Errorf("expected CSRF token %q, got %q", expectedToken, receivedToken)
	}
}

func TestScanTranscripts(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	dir := t.TempDir()

	brainDir := filepath.Join(dir, "brain")
	cascadeID := "conv-1234"
	logDir := filepath.Join(brainDir, cascadeID, ".system_generated", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}

	histPath := filepath.Join(dir, "history.jsonl")
	histLine := `{"conversationId":"` + cascadeID + `","workspace":"/Users/c/awesome-proj"}` + "\n"
	if err := os.WriteFile(histPath, []byte(histLine), 0644); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(dir, "settings.json")
	settingsContent := `{"model":"Gemini 3.8 Flash (Medium)"}`
	if err := os.WriteFile(settingsPath, []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	transcriptPath := filepath.Join(logDir, "transcript.jsonl")
	transcriptContent := strings.Join([]string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-07T10:00:00Z","content":"<USER_SETTINGS_CHANGE>\nThe user changed setting ` + "`Model Selection` from None to Gemini 3.8 Flash (Medium)." + `\n</USER_SETTINGS_CHANGE>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-07T10:00:05Z","input_tokens":5000,"cache_read_tokens":1000,"output_tokens":200}`,
		`{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-07T10:00:10Z","input_tokens":3000,"cache_read_tokens":500,"output_tokens":100}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	sink, usageChan := newTestSink(t, database, calc, "local")
	scanner := &Scanner{
		sink:         sink,
		LogDir:       filepath.Join(dir, "log"),
		HistPath:     histPath,
		BrainDir:     brainDir,
		SettingsPath: settingsPath,
	}

	n, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scanner.Scan: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 records scanned, got %d", n)
	}

	if len(usageChan) != 2 {
		t.Fatalf("expected 2 usage events on chan, got %d", len(usageChan))
	}
	u1 := <-usageChan
	if u1.UUID != "agy:conv-1234:1" || u1.Model != "gemini-3.8-flash" || u1.Project != "awesome-proj" {
		t.Errorf("u1 mismatch: %+v", u1)
	}
	if u1.InputTokens != 6000 || u1.CachedTokens != 1000 || u1.OutputTokens != 200 {
		t.Errorf("u1 tokens mismatch: in=%d cache=%d out=%d", u1.InputTokens, u1.CachedTokens, u1.OutputTokens)
	}

	u2 := <-usageChan
	if u2.UUID != "agy:conv-1234:2" || u2.Model != "gemini-3.8-flash" || u2.Project != "awesome-proj" {
		t.Errorf("u2 mismatch: %+v", u2)
	}

	// Second scan should be incremental (0 new records)
	n2, err := scanner.Scan()
	if err != nil {
		t.Fatalf("second scanner.Scan: %v", err)
	}
	if n2 != 0 {
		t.Errorf("expected 0 new records on second scan, got %d", n2)
	}
}
