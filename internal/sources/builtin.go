package sources

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ai-flight-dashboard/internal/agyscanner"
	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/dsh"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/scanner"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/watcher"
)

const fullScanInterval = time.Minute

// JSONLSource is a built-in line-oriented source.
type JSONLSource struct {
	desc   source.Descriptor
	dirs   func() []string
	runner *scanner.Runner
}

func (s *JSONLSource) Descriptor() source.Descriptor { return s.desc }

func (s *JSONLSource) WatchDirs() []string { return s.dirs() }

func (s *JSONLSource) Scan(ctx context.Context, sink source.Sink) error {
	store, ok := sink.(source.JSONLStore)
	if !ok {
		return nil
	}
	return s.runner.Scan(ctx, store)
}

func (s *JSONLSource) ScanFile(ctx context.Context, path string, sink source.Sink) error {
	store, ok := sink.(source.JSONLStore)
	if !ok {
		return nil
	}
	return s.runner.ScanFile(ctx, path, store)
}

func NewClaudeSource(home string) *JSONLSource {
	dir := filepath.Join(home, ".claude", "projects")
	return newJSONLSource(
		source.Descriptor{ID: "claude-code", Source: "Claude Code", Label: "CLAUDE", Builtin: true},
		[]string{dir},
		newClaudeParser(),
	)
}

func NewGeminiSource(home string) *JSONLSource {
	dir := filepath.Join(home, ".gemini", "tmp")
	return newJSONLSource(
		source.Descriptor{ID: "gemini-cli", Source: "Gemini CLI", Label: "GEMINI", Builtin: true},
		[]string{dir},
		geminiParser{},
	)
}

// NewExtraDirsSource scans user-configured directories. It is hidden from the
// source tabs because its events roll up into the source recorded in each log.
func NewExtraDirsSource() *JSONLSource {
	var mu sync.Mutex
	var cached []string
	var last time.Time
	dirsFn := func() []string {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(last) < 5*time.Second {
			return cached
		}
		cfg, err := config.LoadConfig()
		if err == nil {
			cached = existingDirs(cfg.ExtraWatchDirs)
		}
		last = time.Now()
		return cached
	}
	return &JSONLSource{
		desc: source.Descriptor{ID: "extra-jsonl", Source: "Custom", Label: "CUSTOM", Builtin: true, Hidden: true},
		dirs: dirsFn,
		runner: &scanner.Runner{
			Dirs:         dirsFn,
			Parser:       genericParser{},
			FullInterval: fullScanInterval,
		},
	}
}

func newJSONLSource(desc source.Descriptor, dirs []string, parser source.LineParser) *JSONLSource {
	dirsFn := func() []string { return existingDirs(dirs) }
	s := &JSONLSource{desc: desc, dirs: dirsFn}
	s.runner = &scanner.Runner{
		Dirs:         dirsFn,
		Parser:       parser,
		FullInterval: fullScanInterval,
	}
	return s
}

func existingDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			out = append(out, dir)
		}
	}
	return out
}

// claudeParser derives one request duration from the transcript itself: the
// preceding user/tool_result entry marks the request start, and the assistant
// entry carrying usage marks completion. Assistant snapshots that share a
// message id reuse that same start instead of measuring from one another.
type claudeParser struct {
	mu      sync.Mutex
	states  map[string]*claudeFileState
	current *claudeFileState
}

type claudeFileState struct {
	pendingStart time.Time
	msgStarts    map[string]time.Time
}

func newClaudeParser() *claudeParser {
	return &claudeParser{states: make(map[string]*claudeFileState)}
}

func (p *claudeParser) ResetFile(path string, offset int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.states == nil {
		p.states = make(map[string]*claudeFileState)
	}
	if offset == 0 {
		p.current = &claudeFileState{msgStarts: make(map[string]time.Time)}
		p.states[path] = p.current
		return
	}
	if state, ok := p.states[path]; ok {
		p.current = state
		return
	}
	state := &claudeFileState{msgStarts: make(map[string]time.Time)}
	state.pendingStart = claudePendingStartBefore(path, offset)
	p.states[path] = state
	p.current = state
}

func (p *claudeParser) Parse(line string) (model.TokenUsage, string, bool) {
	project := watcher.ExtractProjectNameFromLine(line)
	entryType, entryTime, messageID := parseClaudeEnvelope(line)

	p.mu.Lock()
	state := p.current
	p.mu.Unlock()

	if state != nil && (entryType == "user" || entryType == "tool_result") && !entryTime.IsZero() {
		p.mu.Lock()
		state.pendingStart = entryTime
		state.msgStarts = make(map[string]time.Time)
		p.mu.Unlock()
	}

	u, ok := watcher.ParseLine(line)
	if !ok || u.Source != "Claude Code" {
		return model.TokenUsage{}, project, false
	}

	if u.OutputTokens > 0 {
		p.mu.Lock()
		if state != nil {
			start := state.pendingStart
			startKey := messageID
			if startKey == "" {
				startKey = u.UUID
			}
			if startKey != "" {
				if known, found := state.msgStarts[startKey]; found {
					start = known
				} else if !start.IsZero() {
					state.msgStarts[startKey] = start
					// Consume the boundary for this response; snapshots sharing
					// the message id still read their start from msgStarts.
					state.pendingStart = time.Time{}
				}
			} else if !start.IsZero() {
				state.pendingStart = time.Time{}
			}
			if !start.IsZero() && !entryTime.IsZero() {
				if duration := entryTime.Sub(start).Milliseconds(); duration > 0 {
					u.DurationMS = duration
					u.TimedOutputTokens = u.OutputTokens
				}
			}
		}
		p.mu.Unlock()
	}
	return u, project, true
}

type claudeEnvelope struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
	} `json:"message"`
}

func parseClaudeEnvelope(line string) (entryType string, at time.Time, messageID string) {
	var env claudeEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return "", time.Time{}, ""
	}
	at = parseClaudeTime(env.Timestamp)
	if at.IsZero() {
		at = parseClaudeTime(env.Message.Timestamp)
	}
	return env.Type, at, env.Message.ID
}

func parseClaudeTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// claudePendingStartBefore recovers the last user/tool_result timestamp when a
// scan resumes after a process restart. It runs once per file, not per tick.
func claudePendingStartBefore(path string, limit int64) time.Time {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var offset int64
	var pending time.Time
	for offset < limit {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if entryType, at, _ := parseClaudeEnvelope(line); (entryType == "user" || entryType == "tool_result") && !at.IsZero() {
				pending = at
			}
			offset += int64(len(line))
		}
		if err != nil {
			break
		}
	}
	return pending
}

func (p *claudeParser) ProjectFromPath(path string) string { return watcher.ExtractProjectName(path) }

func (p *claudeParser) FallbackUUID(string, string, int64, model.TokenUsage) string { return "" }

type geminiParser struct{}

func (geminiParser) Parse(line string) (model.TokenUsage, string, bool) {
	project := watcher.ExtractProjectNameFromLine(line)
	u, ok := watcher.ParseLine(line)
	if !ok || u.Source != "Gemini CLI" {
		return model.TokenUsage{}, project, false
	}
	return u, project, true
}

func (geminiParser) ProjectFromPath(path string) string { return watcher.ExtractProjectName(path) }

func (geminiParser) FallbackUUID(deviceID, path string, lineStartOffset int64, _ model.TokenUsage) string {
	return watcher.StableGeminiUUID(deviceID, path, lineStartOffset)
}

type genericParser struct{}

func (genericParser) Parse(line string) (model.TokenUsage, string, bool) {
	project := watcher.ExtractProjectNameFromLine(line)
	u, ok := watcher.ParseLine(line)
	return u, project, ok
}

func (genericParser) ProjectFromPath(path string) string { return watcher.ExtractProjectName(path) }

func (genericParser) FallbackUUID(deviceID, path string, lineStartOffset int64, u model.TokenUsage) string {
	if u.Source == "Gemini CLI" {
		return watcher.StableGeminiUUID(deviceID, path, lineStartOffset)
	}
	return ""
}

// codexSource wraps the SQLite/session scanner as a poll-only source. It keeps
// one Scanner alive so the thread-table cache and full-scan cadence persist
// between ticks.
type codexSource struct {
	statePath   string
	logsPath    string
	sessionsDir string

	mu      sync.Mutex
	scanner *codexusage.Scanner
}

func NewCodexSource() source.Source {
	home, _ := os.UserHomeDir()
	return &codexSource{
		statePath:   filepath.Join(home, ".codex", "state_5.sqlite"),
		logsPath:    filepath.Join(home, ".codex", "logs_2.sqlite"),
		sessionsDir: filepath.Join(home, ".codex", "sessions"),
	}
}

func (s *codexSource) Descriptor() source.Descriptor {
	return source.Descriptor{ID: "codex", Source: codexusage.SourceName, Label: strings.ToUpper(codexusage.SourceName), Builtin: true}
}

func (s *codexSource) WatchDirs() []string { return nil }

func (s *codexSource) Scan(ctx context.Context, sink source.Sink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scanner == nil {
		s.scanner = codexusage.NewWithSessionPaths(sink, s.statePath, s.logsPath, s.sessionsDir)
	} else {
		s.scanner.SetSink(sink)
	}
	_, err := s.scanner.Scan()
	return err
}

// antigravitySource wraps the Antigravity scanner as a poll-only source. It
// keeps one Scanner alive so metadata caches and the full-scan cadence persist
// between ticks.
type antigravitySource struct {
	mu      sync.Mutex
	scanner *agyscanner.Scanner
}

func NewAntigravitySource() source.Source { return &antigravitySource{} }

func (s *antigravitySource) Descriptor() source.Descriptor {
	return source.Descriptor{ID: "antigravity", Source: "Antigravity", Label: "ANTIGRAVITY", Builtin: true}
}

func (s *antigravitySource) WatchDirs() []string { return nil }

func (s *antigravitySource) Scan(ctx context.Context, sink source.Sink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scanner == nil {
		s.scanner = agyscanner.New(sink)
	} else {
		s.scanner.SetSink(sink)
	}
	_, err := s.scanner.Scan()
	return err
}

// NewDSHSource watches DeepSeek Harness session transcripts, including the
// zstd-compressed v3+ format.
func NewDSHSource(home string) source.Source {
	return dsh.New(home)
}
