package sources

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	p := newClaudeParser()
	line := `{"id":"g1","timestamp":"2026-05-07T11:13:03Z","type":"gemini","tokens":{"input":10,"output":1},"model":"gemini-2.5-pro"}`
	if _, _, ok := p.Parse(line); ok {
		t.Fatal("claude parser must not accept a Gemini record")
	}
}

func TestClaudeParserComputesDurationAndReusesMessageStart(t *testing.T) {
	p := newClaudeParser()
	p.ResetFile("session.jsonl", 0)

	p.Parse(`{"timestamp":"2026-05-07T11:13:00Z","type":"user","message":{"role":"user"}}`)
	first, _, ok := p.Parse(`{"timestamp":"2026-05-07T11:13:02Z","type":"assistant","uuid":"u1","message":{"id":"msg1","type":"message","model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":50}}}`)
	if !ok {
		t.Fatal("expected assistant usage record")
	}
	if first.DurationMS != 2000 || first.TimedOutputTokens != 50 {
		t.Fatalf("unexpected first timing: duration=%d timed=%d", first.DurationMS, first.TimedOutputTokens)
	}
	if first.UUID != "u1" {
		t.Fatalf("parser must preserve the transcript uuid, got %q", first.UUID)
	}

	// A duplicate snapshot of the same response must keep the same request
	// start instead of restarting from its own timestamp.
	duplicate, _, ok := p.Parse(`{"timestamp":"2026-05-07T11:13:02.500Z","type":"assistant","uuid":"u2","message":{"id":"msg1","type":"message","model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":50}}}`)
	if !ok {
		t.Fatal("expected duplicate usage record")
	}
	if duplicate.DurationMS != 2500 || duplicate.TimedOutputTokens != 50 {
		t.Fatalf("unexpected duplicate timing: duration=%d timed=%d", duplicate.DurationMS, duplicate.TimedOutputTokens)
	}

	p.Parse(`{"timestamp":"2026-05-07T11:13:10Z","type":"tool_result","message":{"role":"user"}}`)
	second, _, ok := p.Parse(`{"timestamp":"2026-05-07T11:13:12Z","type":"assistant","uuid":"u3","message":{"id":"msg2","type":"message","model":"claude-opus-4-7","usage":{"input_tokens":20,"output_tokens":80}}}`)
	if !ok {
		t.Fatal("expected second assistant usage record")
	}
	if second.DurationMS != 2000 || second.TimedOutputTokens != 80 {
		t.Fatalf("new tool result must reset the start: duration=%d timed=%d", second.DurationMS, second.TimedOutputTokens)
	}
}

func TestClaudeParserLeavesZeroOutputUntimed(t *testing.T) {
	p := newClaudeParser()
	p.ResetFile("session.jsonl", 0)

	p.Parse(`{"timestamp":"2026-05-07T11:13:00Z","type":"user","message":{"role":"user"}}`)
	u, _, ok := p.Parse(`{"timestamp":"2026-05-07T11:13:02Z","type":"assistant","uuid":"u4","message":{"id":"thinking","type":"message","model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":0}}}`)
	if !ok {
		t.Fatal("expected assistant record")
	}
	if u.DurationMS != 0 || u.TimedOutputTokens != 0 {
		t.Fatalf("zero-output response must not create a rate sample: duration=%d timed=%d", u.DurationMS, u.TimedOutputTokens)
	}
}

func TestClaudeParserRecoversPendingStartAfterRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	userLine := `{"timestamp":"2026-05-07T11:13:00Z","type":"user","message":{"role":"user"}}` + "\n"
	if err := os.WriteFile(path, []byte(userLine), 0o644); err != nil {
		t.Fatal(err)
	}
	offset := int64(len(userLine))
	assistantLine := `{"timestamp":"2026-05-07T11:13:02Z","type":"assistant","model":"claude-opus-4-7","message":{"id":"msg1"},"usage":{"input_tokens":10,"output_tokens":50}}` + "\n"
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(assistantLine); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()

	p := newClaudeParser()
	p.ResetFile(path, offset)
	u, _, ok := p.Parse(assistantLine)
	if !ok {
		t.Fatal("expected assistant usage record")
	}
	if u.DurationMS != 2000 || u.TimedOutputTokens != 50 {
		t.Fatalf("unexpected resumed timing: duration=%d timed=%d", u.DurationMS, u.TimedOutputTokens)
	}
}

func TestClaudePendingStartBeforeIgnoresOldUserAfterLaterBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"timestamp":"2026-05-07T11:13:00Z","type":"user"}` + "\n" +
		`{"timestamp":"2026-05-07T11:13:05Z","type":"tool_result"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := claudePendingStartBefore(path, int64(len(content)))
	want := time.Date(2026, 5, 7, 11, 13, 5, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestClaudePendingStartBeforeResetsAfterCompletedAssistant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"timestamp":"2026-05-07T11:13:00Z","type":"user"}` + "\n" +
		`{"timestamp":"2026-05-07T11:13:02Z","type":"assistant","message":{"id":"msg1","type":"message","stop_reason":"end_turn","model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":50}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := claudePendingStartBefore(path, int64(len(content))); !got.IsZero() {
		t.Fatalf("completed response must not leave a request start, got %s", got)
	}
}

func TestClaudePendingStartBeforeKeepsStartForPartialAssistant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"timestamp":"2026-05-07T11:13:00Z","type":"user"}` + "\n" +
		`{"timestamp":"2026-05-07T11:13:02Z","type":"assistant","message":{"id":"msg1","type":"message","stop_reason":null,"model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":50}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := claudePendingStartBefore(path, int64(len(content)))
	want := time.Date(2026, 5, 7, 11, 13, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("partial response must keep the original start: expected %s, got %s", want, got)
	}
}

func TestGeminiParserRejectsClaudeLines(t *testing.T) {
	p := geminiParser{}
	line := `{"timestamp":"2026-05-07T11:13:03Z","message":{"model":"claude-opus-4-7","type":"message","usage":{"input_tokens":10,"output_tokens":1}}}`
	if _, _, ok := p.Parse(line); ok {
		t.Fatal("gemini parser must not accept a Claude record")
	}
}
