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

	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/watcher"
)

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
	s := &Scanner{
		db:       database,
		calc:     calc,
		DeviceID: "test-device",
		LogDir:   t.TempDir(),
		HistPath: filepath.Join(t.TempDir(), "history.jsonl"),
	}

	ch := make(chan watcher.TokenUsage, 100)
	n, err := s.ScanWithPort(port, ch)
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
	n, err = s.ScanWithPort(port, ch)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if n != 0 {
		t.Fatalf("second scan: expected 0 new records (offset tracking), got %d", n)
	}

	// Add a second step
	currentSteps = []GeneratorMetadata{step1, step2}
	n, err = s.ScanWithPort(port, ch)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("third scan: expected 1 new record (incremental), got %d", n)
	}

	u = <-ch
	if u.InputTokens != 2000 || u.CachedTokens != 500 {
		t.Errorf("third record: input=%d cached=%d, want 2000/500", u.InputTokens, u.CachedTokens)
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

func portFromURL(t *testing.T, url string) int {
	t.Helper()
	parts := strings.Split(url, ":")
	port, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		t.Fatalf("parse port from %q: %v", url, err)
	}
	return port
}
