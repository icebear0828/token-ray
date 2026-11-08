// Package source defines the contract between the dashboard and the tools it
// monitors. Built-in sources and declarative plugin sources both implement
// Source, so the runtime treats them identically.
package source

import (
	"context"

	"ai-flight-dashboard/internal/model"
)

// Descriptor describes a source for the registry and the UI.
type Descriptor struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Builtin     bool   `json:"builtin"`
	Hidden      bool   `json:"hidden,omitempty"`
}

// Source is a tool whose logs can be scanned for token usage.
type Source interface {
	Descriptor() Descriptor
	// WatchDirs returns directories that should receive filesystem events.
	// Sources that only support polling return nil.
	WatchDirs() []string
	// Scan performs one incremental scan. Implementations must be idempotent:
	// re-reading the same data must not create duplicate records.
	Scan(ctx context.Context, sink Sink) error
}

// FileScanner is implemented by sources that can incrementally scan a single
// file. The runtime calls it for filesystem events instead of re-walking the
// whole source.
type FileScanner interface {
	ScanFile(ctx context.Context, path string, sink Sink) error
}

// Sink is the persistence surface available to a source. The host computes
// cost, deduplicates, and broadcasts events; sources only describe usage.
type Sink interface {
	DeviceID() string
	Emit(u model.TokenUsage, filePath string) error
	Offset(key string) (int64, error)
	SetOffset(key string, offset int64) error
	Supersede(source, filePath, deviceID, uuidPrefix string) (int64, error)
}

// JSONLStore is the additional storage surface used by line-oriented scanners.
type JSONLStore interface {
	Sink
	KnownFiles() ([]string, error)
	UpsertKnownDir(dir string) error
}

// LineParser turns one log line into usage. projectHint is returned even when
// the line is not a usage record, so scanners can carry project metadata
// forward within a file.
type LineParser interface {
	Parse(line string) (u model.TokenUsage, projectHint string, ok bool)
	ProjectFromPath(path string) string
	FallbackUUID(deviceID, path string, lineStartOffset int64, u model.TokenUsage) string
}

// FileLineParser is implemented by line parsers that keep state across the
// lines of one file (for example, a model learned from an earlier record).
// The scanner calls ResetFile before reading a file, passing the offset it is
// about to resume from, so parsers can restore per-file state after a restart
// without leaking it between files. Scans are serialized per source, so the
// parser does not need to guard this state for concurrent scans.
type FileLineParser interface {
	LineParser
	ResetFile(path string, offset int64)
}

// UsageIndex is implemented by sinks that can report the usage already stored
// for a file, keyed by UUID. Sources can use it to avoid re-emitting unchanged
// historical records when a transcript is re-read after an append.
type UsageIndex interface {
	KnownUsageByFile(filePath string) (map[string]model.TokenUsage, error)
}

// OffsetIndex is implemented by sinks that can return every recorded offset at
// once. Scanners use it to avoid issuing one query per known file.
type OffsetIndex interface {
	AllOffsets() (map[string]int64, error)
}

// TxRunner is implemented by sinks that can batch writes into one transaction.
// The sink passed to fn is only valid inside the call; its notifications are
// delivered after the transaction commits.
type TxRunner interface {
	RunInTx(fn func(Sink) error) error
}
