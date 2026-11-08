package sources

import (
	"testing"

	"ai-flight-dashboard/internal/model"
)

func TestGeminiParserFallbackUUIDIsStable(t *testing.T) {
	p := geminiParser{}
	a := p.FallbackUUID("device", "/logs/a.jsonl", 10, model.TokenUsage{})
	b := p.FallbackUUID("device", "/logs/a.jsonl", 10, model.TokenUsage{})
	c := p.FallbackUUID("device", "/logs/a.jsonl", 11, model.TokenUsage{})
	if a == "" || a != b {
		t.Fatalf("expected stable uuid, got %q and %q", a, b)
	}
	if a == c {
		t.Fatal("different offsets must produce different uuids")
	}
}

func TestClaudeParserRejectsGeminiLines(t *testing.T) {
	p := claudeParser{}
	line := `{"id":"g1","timestamp":"2026-05-07T11:13:03Z","type":"gemini","tokens":{"input":10,"output":1},"model":"gemini-2.5-pro"}`
	if _, _, ok := p.Parse(line); ok {
		t.Fatal("claude parser must not accept a Gemini record")
	}
}

func TestGeminiParserRejectsClaudeLines(t *testing.T) {
	p := geminiParser{}
	line := `{"timestamp":"2026-05-07T11:13:03Z","message":{"model":"claude-opus-4-7","type":"message","usage":{"input_tokens":10,"output_tokens":1}}}`
	if _, _, ok := p.Parse(line); ok {
		t.Fatal("gemini parser must not accept a Claude record")
	}
}
