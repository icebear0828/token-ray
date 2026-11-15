package watcher_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"token-ray/internal/watcher"
)

func TestWatcher(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	tempDir := t.TempDir()
	err = w.WatchDir(tempDir)
	if err != nil {
		t.Fatalf("failed to watch dir: %v", err)
	}

	logFile := filepath.Join(tempDir, "session.jsonl")

	// Write a Claude log (Trigger Create event)
	claudeLog := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 20}}` + "\n"
	err = os.WriteFile(logFile, []byte(claudeLog), 0644)
	if err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	// Wait for event
	select {
	case usage := <-w.UsageChan:
		if usage.Source != "Claude Code" {
			t.Errorf("Expected source Claude Code, got %s", usage.Source)
		}
		// Claude: input_tokens=100 + cache_read=20 => InputTokens=120, CachedTokens=20
		if usage.InputTokens != 120 || usage.OutputTokens != 50 || usage.CachedTokens != 20 {
			t.Errorf("Tokens parsed incorrectly: %+v", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for watcher event (Claude)")
	}

	// Write a Gemini log by appending (Trigger Write event)
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	geminiLog := `{"type":"gemini", "model": "gemini-2.5-pro", "tokens": {"input": 200, "output": 150, "cached": 50}}` + "\n"
	_, err = file.WriteString(geminiLog)
	file.Close()

	// Wait for second event
	select {
	case usage := <-w.UsageChan:
		if usage.Source != "Gemini CLI" {
			t.Errorf("Expected source Gemini CLI, got %s", usage.Source)
		}
		if usage.InputTokens != 200 || usage.OutputTokens != 150 || usage.CachedTokens != 50 {
			t.Errorf("Tokens parsed incorrectly: %+v", usage)
		}
		if !strings.HasPrefix(usage.UUID, "gemini:") {
			t.Errorf("Expected stable Gemini UUID, got %q", usage.UUID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for second watcher event (Gemini)")
	}
}

func TestRealClaudeCodeFormat(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	tempDir := t.TempDir()
	w.WatchDir(tempDir)

	logFile := filepath.Join(tempDir, "session.jsonl")

	// This is the REAL format Claude Code writes to disk
	realLog := `{"parentUuid":"abc","message":{"model":"claude-sonnet-4-6","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":5000,"cache_creation_input_tokens":200,"cache_read_input_tokens":10000,"output_tokens":1500}}}` + "\n"
	err = os.WriteFile(logFile, []byte(realLog), 0644)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.Source != "Claude Code" {
			t.Errorf("Expected source Claude Code, got %s", usage.Source)
		}
		if usage.Model != "claude-sonnet-4-6" {
			t.Errorf("Expected model claude-sonnet-4-6, got %s", usage.Model)
		}
		// Claude: input_tokens=5000 + cache_read=10000 + cache_creation=200 => InputTokens=15200
		if usage.InputTokens != 15200 {
			t.Errorf("Expected input 15200 (5000+10000+200), got %d", usage.InputTokens)
		}
		if usage.CachedTokens != 10000 {
			t.Errorf("Expected cached 10000, got %d", usage.CachedTokens)
		}
		if usage.CacheCreationTokens != 200 {
			t.Errorf("Expected cache creation 200, got %d", usage.CacheCreationTokens)
		}
		if usage.OutputTokens != 1500 {
			t.Errorf("Expected output 1500, got %d", usage.OutputTokens)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: watcher did not parse real Claude Code JSONL format")
	}
}

func TestWatchDirRecursive(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	// Create nested dir structure simulating ~/.claude/projects/-Users-c-xxx/<uuid>/
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "projects", "-Users-c-myproject")
	subagentDir := filepath.Join(projectDir, "abc-uuid", "subagents")
	os.MkdirAll(subagentDir, 0755)

	err = w.WatchDirRecursive(tempDir)
	if err != nil {
		t.Fatalf("failed to watch dir recursive: %v", err)
	}

	// Write jsonl into a deeply nested subdir (simulates real Claude log location)
	logFile := filepath.Join(projectDir, "abc-uuid.jsonl")
	// Claude: input=500 + cache_read=100 => InputTokens=600
	claudeLog := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 500, "output_tokens": 200, "cache_read_input_tokens": 100}}` + "\n"
	err = os.WriteFile(logFile, []byte(claudeLog), 0644)
	if err != nil {
		t.Fatalf("failed to write nested log: %v", err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.InputTokens != 600 || usage.OutputTokens != 200 || usage.CachedTokens != 100 {
			t.Errorf("Nested dir tokens wrong: %+v (expected InputTokens=600)", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: recursive watcher did not pick up nested .jsonl")
	}

	// Also write into subagents/ dir
	subLog := filepath.Join(subagentDir, "agent-xxx.jsonl")
	// Claude: input=300 + cache_read=0 => InputTokens=300
	subEntry := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 300, "output_tokens": 100, "cache_read_input_tokens": 0}}` + "\n"
	err = os.WriteFile(subLog, []byte(subEntry), 0644)
	if err != nil {
		t.Fatalf("failed to write subagent log: %v", err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.InputTokens != 300 || usage.OutputTokens != 100 {
			t.Errorf("Subagent dir tokens wrong: %+v", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: recursive watcher did not pick up subagent .jsonl")
	}
}

func TestWatchDirRecursive_NewSubdir(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	tempDir := t.TempDir()
	err = w.WatchDirRecursive(tempDir)
	if err != nil {
		t.Fatalf("failed to watch: %v", err)
	}

	// Create a NEW subdirectory after watcher started (simulates new Claude session)
	newSessionDir := filepath.Join(tempDir, "new-session-uuid")
	os.MkdirAll(newSessionDir, 0755)
	// Give fsnotify time to auto-register the new subdir
	time.Sleep(200 * time.Millisecond)

	logFile := filepath.Join(newSessionDir, "session.jsonl")
	// Claude: input=999 + cache_read=0 => InputTokens=999
	entry := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 999, "output_tokens": 111, "cache_read_input_tokens": 0}}` + "\n"
	err = os.WriteFile(logFile, []byte(entry), 0644)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.InputTokens != 999 || usage.OutputTokens != 111 {
			t.Errorf("Dynamic subdir tokens wrong: %+v", usage)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout: watcher did not auto-register dynamically created subdir")
	}
}

func TestParseLine_GeminiWithThoughtsAndTool(t *testing.T) {
	// Real Gemini CLI format with thoughts and tool tokens
	line := `{"id":"abc-123","timestamp":"2026-05-01T02:44:45.432Z","type":"gemini","content":"hello","tokens":{"input":8864,"output":52,"cached":3000,"thoughts":576,"tool":100,"total":9592},"model":"gemini-3.1-pro-preview"}` + "\n"

	u, ok := watcher.ParseLine(line)
	if !ok {
		t.Fatal("ParseLine should have succeeded for Gemini format")
	}

	if u.Source != "Gemini CLI" {
		t.Errorf("Expected source 'Gemini CLI', got %q", u.Source)
	}
	if u.Model != "gemini-3.1-pro-preview" {
		t.Errorf("Expected model 'gemini-3.1-pro-preview', got %q", u.Model)
	}
	if u.InputTokens != 8864 {
		t.Errorf("Expected InputTokens=8864, got %d", u.InputTokens)
	}
	if u.CachedTokens != 3000 {
		t.Errorf("Expected CachedTokens=3000, got %d", u.CachedTokens)
	}
	// OutputTokens = output + thoughts + tool = 52 + 576 + 100 = 728
	if u.OutputTokens != 728 {
		t.Errorf("Expected OutputTokens=728 (52+576+100), got %d", u.OutputTokens)
	}
	if u.Thoughts != 576 {
		t.Errorf("Expected Thoughts=576, got %d", u.Thoughts)
	}
	if u.UUID != "abc-123" {
		t.Errorf("Expected UUID 'abc-123', got %q", u.UUID)
	}
	if u.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

func TestParseLine_ClaudeUsesCWDProject(t *testing.T) {
	line := `{"uuid":"abc","cwd":"/Users/john-doe/my-hyphen-app","timestamp":"2026-05-01T02:44:45Z","message":{"model":"claude-sonnet-4-6","type":"message","role":"assistant","usage":{"input_tokens":100,"output_tokens":20}}}` + "\n"

	u, ok := watcher.ParseLine(line)
	if !ok {
		t.Fatal("ParseLine should have succeeded")
	}

	if u.Project != "my-hyphen-app" {
		t.Errorf("expected cwd-derived project my-hyphen-app, got %q", u.Project)
	}
}

func TestWithProjectFallbackDoesNotOverrideMetadataProject(t *testing.T) {
	u := watcher.TokenUsage{Project: "my-hyphen-app"}
	u = watcher.WithProjectFallback(u, "doe-my-hyphen-app")

	if u.Project != "my-hyphen-app" {
		t.Errorf("fallback overwrote metadata project: got %q", u.Project)
	}
}

func TestExtractProjectNameFromLineReadsNonUsageCWD(t *testing.T) {
	line := `{"type":"system","cwd":"/Users/john-doe/my-hyphen-app"}` + "\n"

	got := watcher.ExtractProjectNameFromLine(line)
	if got != "my-hyphen-app" {
		t.Errorf("expected cwd-derived project my-hyphen-app, got %q", got)
	}
}

func TestExtractProjectName_GeminiPaths(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/Users/c/.gemini/tmp/token/chats/session-2026-05-01.jsonl", "token"},
		{"/Users/c/.gemini/tmp/codex-proxy/chats/session-abc.jsonl", "codex-proxy"},
		{"/Users/c/.gemini/tmp/wiki/chats/session.jsonl", "wiki"},
		{"/Users/c/.gemini/other/something.jsonl", "Gemini"}, // fallback for non-tmp paths
		{"/Users/c/.claude/projects/-Users-c-myapp/abc.jsonl", "myapp"},
		{"/Users/c/.claude/projects/-Users-c/abc.jsonl", "c"},
		{"/Users/c/.claude/projects/Users-c-antigravity-proxy/abc.jsonl", "antigravity-proxy"},
		{"/Users/alice/.claude/projects/-Users-alice-my-hyphen-app/abc.jsonl", "my-hyphen-app"},
		{"/Users/alice/.claude/projects/-Users-alice/abc.jsonl", "alice"},
		{"/Users/runner/.claude/projects/-Users-runner-service-api/abc.jsonl", "service-api"},
		{"/home/bob/.claude/projects/-home-bob-service-api/abc.jsonl", "service-api"},
		{"/home/bob/.claude/projects/-home-bob/abc.jsonl", "bob"},
		{`C:\Users\alice\.claude\projects\C:-Users-alice-windows-app\abc.jsonl`, "windows-app"},
		{`C:\Users\alice\.claude\projects\C:-Users-alice\abc.jsonl`, "alice"},
		{"/some/random/path/data.jsonl", "Default"},
	}
	for _, tt := range tests {
		got := watcher.ExtractProjectName(tt.path)
		if got != tt.want {
			t.Errorf("ExtractProjectName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtractProjectName_GeminiUsesProjectRootFile(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, ".gemini", "tmp", "encoded-folder")
	chatsDir := filepath.Join(projectDir, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".project_root"), []byte("/Users/alice/wiki-token\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.Join(chatsDir, "session.jsonl")
	got := watcher.ExtractProjectName(logFile)
	if got != "wiki-token" {
		t.Errorf("expected project_root-derived project wiki-token, got %q", got)
	}
}

func TestExtractProjectNameFromCWD_NormalizesWindowsPath(t *testing.T) {
	got := watcher.ExtractProjectNameFromCWD(`C:\Users\john-doe\my-hyphen-app\`)
	if got != "my-hyphen-app" {
		t.Errorf("expected Windows cwd base my-hyphen-app, got %q", got)
	}
}

func TestParseLine_GeminiUsesTotalForUnknownTokenClasses(t *testing.T) {
	line := `{"id":"abc-123","timestamp":"2026-05-01T02:44:45.432Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250,"thoughts":25,"tool":5,"total":1200},"model":"gemini-3.1-pro-preview"}` + "\n"

	u, ok := watcher.ParseLine(line)
	if !ok {
		t.Fatal("ParseLine should have succeeded")
	}

	if u.InputTokens != 1000 {
		t.Errorf("expected input 1000, got %d", u.InputTokens)
	}
	if u.CachedTokens != 250 {
		t.Errorf("expected cached 250, got %d", u.CachedTokens)
	}
	if u.OutputTokens != 200 {
		t.Errorf("expected output to reconcile to total-input=200, got %d", u.OutputTokens)
	}
}
