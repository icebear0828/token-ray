package plugin

import (
	"strings"
	"testing"
	"time"

	"ai-flight-dashboard/internal/model"
)

func testParser() *parser {
	return &parser{manifest: Manifest{
		SchemaVersion: 1,
		ID:            "demo",
		Source:        "Demo",
		Version:       "1.0.0",
		WatchDirs:     []string{"~/.demo/logs"},
		Match:         &Match{Field: "kind", Equals: "usage"},
		Fields: FieldMap{
			Model:               "model",
			InputTokens:         "usage.input",
			CachedTokens:        "usage.cached",
			CacheCreationTokens: "usage.cache_write",
			OutputTokens:        "usage.output",
			Thoughts:            "usage.thoughts",
			Timestamp:           "ts",
			UUID:                "id",
			Project:             "cwd",
		},
	}}
}

func TestParserMapsFields(t *testing.T) {
	line := `{"kind":"usage","id":"u1","model":"gpt-x","ts":"2026-01-02T03:04:05Z","cwd":"/work/my-proj","usage":{"input":100,"cached":20,"cache_write":5,"output":10,"thoughts":3}}`
	u, project, ok := testParser().Parse(line)
	if !ok {
		t.Fatal("expected usage line to parse")
	}
	if u.Source != "Demo" || u.Model != "gpt-x" {
		t.Fatalf("unexpected identity: %+v", u)
	}
	if u.InputTokens != 100 || u.CachedTokens != 20 || u.CacheCreationTokens != 5 || u.OutputTokens != 10 || u.Thoughts != 3 {
		t.Fatalf("unexpected tokens: %+v", u)
	}
	if u.UUID != "u1" {
		t.Fatalf("unexpected uuid: %q", u.UUID)
	}
	if project != "my-proj" || u.Project != "my-proj" {
		t.Fatalf("unexpected project: hint=%q project=%q", project, u.Project)
	}
	if !u.Timestamp.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("unexpected timestamp: %v", u.Timestamp)
	}
}

func TestParserExclusiveInputMode(t *testing.T) {
	p := testParser()
	p.manifest.InputMode = "exclusive"
	line := `{"kind":"usage","model":"gpt-x","usage":{"input":100,"cached":20,"output":10}}`
	u, _, ok := p.Parse(line)
	if !ok {
		t.Fatal("expected usage line to parse")
	}
	if u.InputTokens != 120 {
		t.Fatalf("expected cached tokens folded into input, got %d", u.InputTokens)
	}
}

func TestParserMatchFiltersLines(t *testing.T) {
	p := testParser()
	if _, _, ok := p.Parse(`{"kind":"other","usage":{"input":1}}`); ok {
		t.Fatal("expected non-matching line to be skipped")
	}
}

func TestParserRejectsNoise(t *testing.T) {
	p := testParser()
	if _, _, ok := p.Parse(`{"kind":"usage","model":"gpt-x","usage":{}}`); ok {
		t.Fatal("expected zero-token record to be skipped")
	}
}

func TestParserFallbackUUIDIsStable(t *testing.T) {
	p := testParser()
	first := p.FallbackUUID("device", "/logs/a.jsonl", 42, model.TokenUsage{})
	second := p.FallbackUUID("device", "/logs/a.jsonl", 42, model.TokenUsage{})
	other := p.FallbackUUID("device", "/logs/a.jsonl", 43, model.TokenUsage{})
	if first == "" || first != second {
		t.Fatalf("expected stable fallback uuid, got %q and %q", first, second)
	}
	if first == other {
		t.Fatal("different offsets must produce different uuids")
	}
	if !strings.HasPrefix(first, "plugin:demo:") {
		t.Fatalf("unexpected uuid shape: %q", first)
	}
}

func TestParserUnixTimestamp(t *testing.T) {
	p := testParser()
	line := `{"kind":"usage","model":"gpt-x","ts":1767323045,"usage":{"input":1,"output":1}}`
	u, _, ok := p.Parse(line)
	if !ok {
		t.Fatal("expected usage line to parse")
	}
	if u.Timestamp.IsZero() || u.Timestamp.Year() < 2025 {
		t.Fatalf("unexpected timestamp: %v", u.Timestamp)
	}
}

func TestParserStripsBOM(t *testing.T) {
	p := testParser()
	line := string(rune(0xFEFF)) + `{"kind":"usage","model":"gpt-x","usage":{"input":1,"output":1}}`
	if _, _, ok := p.Parse(line); !ok {
		t.Fatal("expected BOM-prefixed line to parse")
	}
}
