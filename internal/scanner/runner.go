package scanner

import (
	"context"
	"sync"
	"time"

	"ai-flight-dashboard/internal/source"
)

// Runner owns the incremental-scan cadence for one line-oriented source:
// cheap known-file scans on every tick, with a full directory walk every
// FullInterval. It also serializes scans so overlapping filesystem events
// cannot read the same file twice.
type Runner struct {
	Dirs         func() []string
	Parser       source.LineParser
	Match        func(string) bool
	FullInterval time.Duration

	mu       sync.Mutex
	scanMu   sync.Mutex
	lastFull time.Time
}

func (r *Runner) Scan(ctx context.Context, sink source.JSONLStore) error {
	r.scanMu.Lock()
	defer r.scanMu.Unlock()

	full := r.shouldFullScan()
	var dirs []string
	if r.Dirs != nil {
		dirs = r.Dirs()
	}
	s := NewWithParser(sink, r.Parser, r.Match)
	if full {
		_, err := s.ScanAll(dirs)
		return err
	}
	_, err := s.ScanKnownFiles(dirs)
	return err
}

func (r *Runner) ScanFile(ctx context.Context, path string, sink source.JSONLStore) error {
	r.scanMu.Lock()
	defer r.scanMu.Unlock()

	s := NewWithParser(sink, r.Parser, r.Match)
	_, err := s.ScanFile(path)
	return err
}

func (r *Runner) shouldFullScan() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	interval := r.FullInterval
	if interval <= 0 {
		interval = time.Minute
	}
	if r.lastFull.IsZero() || time.Since(r.lastFull) >= interval {
		r.lastFull = time.Now()
		return true
	}
	return false
}
