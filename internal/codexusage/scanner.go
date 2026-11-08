package codexusage

import (
	"os"
	"path/filepath"

	"ai-flight-dashboard/internal/source"
)

const (
	SourceName = "Codex"
	OffsetKey  = "codex:logs_2.sqlite:id"
)

type Scanner struct {
	sink        source.Sink
	StatePath   string
	LogsPath    string
	SessionsDir string
}

type threadInfo struct {
	model string
	cwd   string
}

func New(sink source.Sink) *Scanner {
	home, _ := os.UserHomeDir()
	return &Scanner{
		sink:        sink,
		StatePath:   filepath.Join(home, ".codex", "state_5.sqlite"),
		LogsPath:    filepath.Join(home, ".codex", "logs_2.sqlite"),
		SessionsDir: filepath.Join(home, ".codex", "sessions"),
	}
}

func NewWithPaths(sink source.Sink, statePath string, logsPath string) *Scanner {
	return &Scanner{sink: sink, StatePath: statePath, LogsPath: logsPath}
}

func NewWithSessionPaths(sink source.Sink, statePath string, logsPath string, sessionsDir string) *Scanner {
	return &Scanner{sink: sink, StatePath: statePath, LogsPath: logsPath, SessionsDir: sessionsDir}
}

// Scan imports Codex usage from session JSONL totals when available, falling
// back to response.completed telemetry from ~/.codex/logs_2.sqlite.
func (s *Scanner) Scan() (int, error) {
	threads := make(map[string]threadInfo)
	if s.StatePath != "" {
		if _, err := os.Stat(s.StatePath); err == nil {
			loaded, ok, err := loadThreads(s.StatePath)
			if err != nil {
				return 0, err
			}
			if ok {
				threads = loaded
			}
		}
	}

	sessionCount, hasSessionTotals, err := s.scanSessions(threads)
	if err != nil {
		return sessionCount, err
	}
	if hasSessionTotals {
		if _, err := s.sink.Supersede(SourceName, s.LogsPath, s.sink.DeviceID(), "codex:"); err != nil {
			return sessionCount, err
		}
		return sessionCount, nil
	}

	logCount, err := s.scanLogs(threads)
	return sessionCount + logCount, err
}
