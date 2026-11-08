package sources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ai-flight-dashboard/internal/agyscanner"
	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/config"
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
		claudeParser{},
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

type claudeParser struct{}

func (claudeParser) Parse(line string) (model.TokenUsage, string, bool) {
	project := watcher.ExtractProjectNameFromLine(line)
	u, ok := watcher.ParseLine(line)
	if !ok || u.Source != "Claude Code" {
		return model.TokenUsage{}, project, false
	}
	return u, project, true
}

func (claudeParser) ProjectFromPath(path string) string { return watcher.ExtractProjectName(path) }

func (claudeParser) FallbackUUID(string, string, int64, model.TokenUsage) string { return "" }

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

// codexSource wraps the SQLite/session scanner as a poll-only source.
type codexSource struct {
	statePath   string
	logsPath    string
	sessionsDir string
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
	_, err := codexusage.NewWithSessionPaths(sink, s.statePath, s.logsPath, s.sessionsDir).Scan()
	return err
}

// antigravitySource wraps the Antigravity scanner as a poll-only source.
type antigravitySource struct{}

func NewAntigravitySource() source.Source { return &antigravitySource{} }

func (s *antigravitySource) Descriptor() source.Descriptor {
	return source.Descriptor{ID: "antigravity", Source: "Antigravity", Label: "ANTIGRAVITY", Builtin: true}
}

func (s *antigravitySource) WatchDirs() []string { return nil }

func (s *antigravitySource) Scan(ctx context.Context, sink source.Sink) error {
	_, err := agyscanner.New(sink).Scan()
	return err
}
