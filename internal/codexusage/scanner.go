package codexusage

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"ai-flight-dashboard/internal/source"
)

const (
	SourceName = "Codex"
	OffsetKey  = "codex:logs_2.sqlite:id"

	sessionOffsetPrefix = "codex:session:"
)

type Scanner struct {
	sink        source.Sink
	StatePath   string
	LogsPath    string
	SessionsDir string

	mu           sync.Mutex
	lastFull     time.Time
	fullInterval time.Duration

	threadsMu    sync.Mutex
	threads      map[string]threadInfo
	threadsSig   fileSignature
	threadsValid bool
}

type threadInfo struct {
	model string
	cwd   string
}

// fileSignature identifies a SQLite database state. The -wal file is included
// because a WAL-mode database can change without its main file's mtime moving.
type fileSignature struct {
	modTime time.Time
	size    int64
	walTime time.Time
	walSize int64
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

// SetSink updates the target sink so a long-lived Scanner remains attached to
// the active sink instance when callers pass a new one into Scan.
func (s *Scanner) SetSink(sink source.Sink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink = sink
}

// Scan imports Codex usage from session JSONL totals when available, falling
// back to response.completed telemetry from ~/.codex/logs_2.sqlite. Session
// directories are walked at most once per fullInterval; the ticks in between
// only stat the session files already recorded in scan_offsets.
func (s *Scanner) Scan() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	threads, err := s.loadThreadsCached()
	if err != nil {
		return 0, err
	}

	offsets, hasOffsets := s.allOffsets()
	full := s.shouldFullScan()
	if !hasOffsets {
		full = true
	}

	sessionCount, hasSessionTotals, err := s.scanSessions(threads, full, offsets)
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

func (s *Scanner) shouldFullScan() bool {
	interval := s.fullInterval
	if interval <= 0 {
		interval = time.Minute
	}
	if s.lastFull.IsZero() || time.Since(s.lastFull) >= interval {
		s.lastFull = time.Now()
		return true
	}
	return false
}

func (s *Scanner) allOffsets() (map[string]int64, bool) {
	index, ok := s.sink.(source.OffsetIndex)
	if !ok {
		return nil, false
	}
	offsets, err := index.AllOffsets()
	if err != nil {
		return nil, false
	}
	return offsets, true
}

func (s *Scanner) offsetFor(offsets map[string]int64, key string) (int64, error) {
	if offsets != nil {
		return offsets[key], nil
	}
	return s.sink.Offset(key)
}

// loadThreadsCached reuses the thread table until the state database (or its
// WAL file) changes, so a 2s poll does not re-read and deserialize every row.
func (s *Scanner) loadThreadsCached() (map[string]threadInfo, error) {
	if s.StatePath == "" {
		return nil, nil
	}
	sig, ok := readFileSignature(s.StatePath)
	if !ok {
		return nil, nil
	}

	s.threadsMu.Lock()
	defer s.threadsMu.Unlock()
	if s.threadsValid && s.threadsSig == sig {
		return s.threads, nil
	}
	loaded, has, err := loadThreads(s.StatePath)
	if err != nil {
		return nil, err
	}
	if !has {
		loaded = nil
	}
	s.threads = loaded
	s.threadsSig = sig
	s.threadsValid = true
	return loaded, nil
}

func readFileSignature(path string) (fileSignature, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileSignature{}, false
	}
	sig := fileSignature{modTime: info.ModTime(), size: info.Size()}
	if wal, err := os.Stat(path + "-wal"); err == nil {
		sig.walTime = wal.ModTime()
		sig.walSize = wal.Size()
	}
	return sig, true
}
