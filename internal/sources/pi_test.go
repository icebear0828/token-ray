package sources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func TestPiParserMapsUsageAndCarriesModel(t *testing.T) {
	parser := newPiParser()
	parser.ResetFile(filepath.Join("sessions", "proj", "2026-10-12T02-27-31-079Z_session-1.jsonl"), 0)

	var project string
	var usages []model.TokenUsage
	lines := []string{
		`{"type":"session","id":"session-1","cwd":"D:\\work\\Demo"}`,
		`{"type":"model_change","provider":"deepseek","modelId":"deepseek-v4-pro"}`,
		`{"type":"message","id":"m1","timestamp":"2026-10-12T02:28:43.068Z","message":{"role":"assistant","usage":{"input":100,"output":20,"cacheRead":50,"cacheWrite":10,"reasoning":5}}}`,
		`{"type":"compaction","timestamp":"2026-10-12T02:28:44.000Z","usage":{"input":40,"output":10}}`,
	}
	for _, line := range lines {
		usage, hint, ok := parser.Parse(line)
		if hint != "" {
			project = hint
		}
		if ok {
			usages = append(usages, usage)
		}
	}

	if project != "Demo" {
		t.Fatalf("expected project Demo, got %q", project)
	}
	if len(usages) != 2 {
		t.Fatalf("expected two usage records, got %+v", usages)
	}
	first := usages[0]
	if first.Source != piSourceName || first.Model != "deepseek-v4-pro" {
		t.Fatalf("unexpected first source/model: %+v", first)
	}
	if first.InputTokens != 160 || first.CachedTokens != 50 || first.CacheCreationTokens != 10 || first.OutputTokens != 20 {
		t.Fatalf("unexpected first token mapping: %+v", first)
	}
	if first.Thoughts != 5 {
		t.Fatalf("reasoning should be informational, got %+v", first)
	}
	if first.UUID != "pi-session-1-m1" {
		t.Fatalf("unexpected uuid: %q", first.UUID)
	}
	if first.Timestamp.IsZero() {
		t.Fatal("expected a parsed timestamp")
	}
	second := usages[1]
	if second.Model != "deepseek-v4-pro" || second.InputTokens != 40 || second.OutputTokens != 10 {
		t.Fatalf("model should carry across lines: %+v", second)
	}
}

func TestPiParserResetFileClearsModel(t *testing.T) {
	parser := newPiParser()
	parser.ResetFile("a/2026-10-12T02-27-31-079Z_a.jsonl", 0)
	if _, _, ok := parser.Parse(`{"type":"model_change","modelId":"deepseek-v4-pro"}`); ok {
		t.Fatal("model_change must not emit usage")
	}
	parser.ResetFile("b/2026-10-12T02-27-31-079Z_b.jsonl", 0)
	usage, _, ok := parser.Parse(`{"type":"message","id":"m1","timestamp":"2026-10-12T02:28:43.068Z","message":{"role":"assistant","usage":{"input":1,"output":1}}}`)
	if !ok {
		t.Fatal("expected assistant usage")
	}
	if usage.Model != "unknown" {
		t.Fatalf("expected model state to reset, got %q", usage.Model)
	}
}

func TestPiParserRestoresStateFromPrefix(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-10-12T02-27-31-079Z_session-1.jsonl")
	lines := []string{
		`{"type":"session","id":"session-1","cwd":"D:\\work\\Demo"}`,
		`{"type":"model_change","modelId":"deepseek-v4-pro"}`,
		`{"type":"message","id":"m1","timestamp":"2026-10-12T02:28:43.068Z","message":{"role":"assistant","usage":{"input":100,"output":20}}}`,
	}
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a process restart with a persisted offset: the parser has no
	// in-memory state and must recover it from the file prefix.
	parser := newPiParser()
	parser.ResetFile(file, info.Size())
	if project := parser.ProjectFromPath(file); project != "Demo" {
		t.Fatalf("expected project from prefix, got %q", project)
	}

	usage, _, ok := parser.Parse(`{"type":"message","id":"m2","timestamp":"2026-10-12T02:29:00.000Z","message":{"role":"assistant","usage":{"input":30,"output":5}}}`)
	if !ok {
		t.Fatal("expected appended assistant usage")
	}
	if usage.Model != "deepseek-v4-pro" {
		t.Fatalf("expected model from prefix, got %q", usage.Model)
	}
}

func TestPiProjectFromDir(t *testing.T) {
	if got := piProjectFromDir("/x/sessions/--D--project--/file.jsonl"); got != "project" {
		t.Fatalf("expected project, got %q", got)
	}
	if got := piProjectFromDir("/x/sessions/--Users--c--myproject--/file.jsonl"); got != "myproject" {
		t.Fatalf("expected myproject, got %q", got)
	}
}

func TestPiFallbackUUIDIsStable(t *testing.T) {
	parser := newPiParser()
	a := parser.FallbackUUID("device", "/logs/a.jsonl", 10, model.TokenUsage{})
	b := parser.FallbackUUID("device", "/logs/a.jsonl", 10, model.TokenUsage{})
	c := parser.FallbackUUID("device", "/logs/a.jsonl", 11, model.TokenUsage{})
	if a == "" || a != b {
		t.Fatalf("expected stable uuid, got %q and %q", a, b)
	}
	if a == c {
		t.Fatal("different offsets must produce different uuids")
	}
}

func TestPiSourceScansSessionsAndDedups(t *testing.T) {
	agentDir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)

	dir := filepath.Join(agentDir, "sessions", "--D--project--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "2026-10-12T02-27-31-079Z_sid-1.jsonl")
	lines := []string{
		`{"type":"session","id":"sid-1","cwd":"D:\\project"}`,
		`{"type":"model_change","modelId":"deepseek-v4-pro"}`,
		`{"type":"message","id":"m1","timestamp":"2026-10-12T02:28:43.068Z","message":{"role":"assistant","usage":{"input":100,"output":20,"cacheRead":50,"cacheWrite":10}}}`,
		`{"type":"compaction","timestamp":"2026-10-12T02:28:44.000Z","usage":{"input":10,"output":2}}`,
	}
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, calc := testutil.NewTestDBAndCalc(t)
	sink := testutil.NewTestSink(t, database, calc, "local")
	src := NewPiSource(t.TempDir())
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Model != "deepseek-v4-pro" || stats[0].Events != 2 {
		t.Fatalf("unexpected model stat after first scan: %+v", stats)
	}
	if stats[0].InputTokens != 170 || stats[0].CachedTokens != 50 || stats[0].CacheCreationTokens != 10 || stats[0].OutputTokens != 22 {
		t.Fatalf("unexpected token totals after first scan: %+v", stats[0])
	}
	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "project" || projects[0].Events != 2 {
		t.Fatalf("expected project attribution, got %+v", projects)
	}

	// Append a turn without re-writing the session header or model_change.
	appended := `{"type":"message","id":"m2","timestamp":"2026-10-12T02:29:00.000Z","message":{"role":"assistant","usage":{"input":30,"output":5}}}` + "\n"
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"+appended), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err = database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Model != "deepseek-v4-pro" || stats[0].Events != 3 {
		t.Fatalf("appended turn lost its model state: %+v", stats)
	}
	if stats[0].InputTokens != 200 || stats[0].OutputTokens != 27 {
		t.Fatalf("unexpected token totals after append: %+v", stats[0])
	}
	projects, err = database.QueryProjectStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "project" || projects[0].Events != 3 {
		t.Fatalf("appended turn lost its project state: %+v", projects)
	}

	// A full re-read must not duplicate the compaction record, which has no
	// message id and relies on the stable fallback UUID.
	if _, err := database.ResetOffset(file); err != nil {
		t.Fatal(err)
	}
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err = database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 3 {
		t.Fatalf("rescan must not duplicate rows, got %+v", stats)
	}
}
