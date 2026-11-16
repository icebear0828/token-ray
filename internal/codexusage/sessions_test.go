package codexusage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCodexSessionMeasuresPerCallDuration(t *testing.T) {
	lines := []string{
		`{"timestamp":"2026-05-07T11:13:00.000Z","type":"session_meta","payload":{"id":"s1","cwd":"/work/demo","model":"gpt-5.6"}}`,
		`{"timestamp":"2026-05-07T11:13:00.000Z","type":"event_msg","payload":{"type":"task_started"}}`,
		`{"timestamp":"2026-05-07T11:13:00.100Z","type":"event_msg","payload":{"type":"user_message","message":"hi"}}`,
		`{"timestamp":"2026-05-07T11:13:00.200Z","type":"response_item","payload":{"type":"reasoning"}}`,
		`{"timestamp":"2026-05-07T11:13:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":50},"last_token_usage":{"input_tokens":10,"output_tokens":50}}}}`,
		`{"timestamp":"2026-05-07T11:13:02.100Z","type":"response_item","payload":{"type":"function_call"}}`,
		`{"timestamp":"2026-05-07T11:13:03.000Z","type":"response_item","payload":{"type":"function_call_output"}}`,
		`{"timestamp":"2026-05-07T11:13:03.100Z","type":"response_item","payload":{"type":"reasoning"}}`,
		`{"timestamp":"2026-05-07T11:13:05.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":20,"output_tokens":80},"last_token_usage":{"input_tokens":10,"output_tokens":30}}}}`,
		`{"timestamp":"2026-05-07T11:13:05.100Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":20,"output_tokens":80},"last_token_usage":{"input_tokens":10,"output_tokens":30}}}}`,
		`{"timestamp":"2026-05-07T11:13:05.200Z","type":"event_msg","payload":{"type":"task_complete"}}`,
	}
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	total, err := parseCodexSessionFile(path, time.Date(2026, 5, 7, 11, 13, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if total.id != "s1" || total.model != "gpt-5.6" || total.cwd != "/work/demo" {
		t.Fatalf("unexpected metadata: %+v", total)
	}
	if total.tokens.OutputTokens != 80 {
		t.Fatalf("expected cumulative output 80, got %d", total.tokens.OutputTokens)
	}
	if total.timedOutput != 80 {
		t.Fatalf("expected timed output 80, got %d", total.timedOutput)
	}
	if total.durationMS != 3900 {
		t.Fatalf("expected 3900ms of measured generation, got %d", total.durationMS)
	}
}
