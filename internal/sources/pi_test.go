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
	parser := &piParser{}
	parser.ResetFile(filepath.Join("sessions", "proj", "2026-10-12T02-27-31-079Z_session-1.jsonl"))

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
	parser := &piParser{}
	parser.ResetFile("a/2026-10-12T02-27-31-079Z_a.jsonl")
	if _, _, ok := parser.Parse(`{"type":"model_change","modelId":"deepseek-v4-pro"}`); ok {
		t.Fatal("model_change must not emit usage")
	}
	parser.ResetFile("b/2026-10-12T02-27-31-079Z_b.jsonl")
	usage, _, ok := parser.Parse(`{"type":"message","id":"m1","timestamp":"2026-10-12T02:28:43.068Z","message":{"role":"assistant","usage":{"input":1,"output":1}}}`)
	if !ok {
		t.Fatal("expected assistant usage")
	}
	if usage.Model != "unknown" {
		t.Fatalf("expected model state to reset, got %q", usage.Model)
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
	if len(stats) != 1 {
		t.Fatalf("expected one model stat, got %+v", stats)
	}
	if stats[0].Model != "deepseek-v4-pro" || stats[0].Events != 1 {
		t.Fatalf("unexpected model stat: %+v", stats[0])
	}
	if stats[0].InputTokens != 160 || stats[0].CachedTokens != 50 || stats[0].CacheCreationTokens != 10 || stats[0].OutputTokens != 20 {
		t.Fatalf("unexpected token totals: %+v", stats[0])
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "project" {
		t.Fatalf("expected project attribution, got %+v", projects)
	}

	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err = database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 1 {
		t.Fatalf("rescan must not duplicate rows, got %+v", stats)
	}
}
