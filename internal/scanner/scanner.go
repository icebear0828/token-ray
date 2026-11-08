package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/watcher"
)

// Scanner reads line-oriented log files incrementally and emits usage through
// a source.Sink. It is shared by built-in sources and declarative plugins.
type Scanner struct {
	sink   source.JSONLStore
	parser source.LineParser
	match  func(string) bool
}

// New creates a Scanner for JSONL files using the default Claude/Gemini parser.
func New(sink source.JSONLStore) *Scanner {
	return NewWithParser(sink, defaultParser{}, isJSONL)
}

// NewWithParser creates a Scanner with a custom parser and file matcher.
func NewWithParser(sink source.JSONLStore, parser source.LineParser, match func(string) bool) *Scanner {
	if match == nil {
		match = isJSONL
	}
	return &Scanner{sink: sink, parser: parser, match: match}
}

func isJSONL(path string) bool {
	return strings.HasSuffix(path, ".jsonl")
}

// scanEntry is one parsed usage record waiting to be written.
type scanEntry struct {
	u  model.TokenUsage
	ts time.Time
}

// ScanAll walks all dirs, finds matching files, reads from the last offset,
// parses usage, and emits it. Returns the number of new usage records.
func (s *Scanner) ScanAll(dirs []string) (int, error) {
	return s.scanAll(dirs, false)
}

// ScanAllStrict is used by repair flows where silently skipping a replay file
// could leave repaired history incomplete.
func (s *Scanner) ScanAllStrict(dirs []string) (int, error) {
	return s.scanAll(dirs, true)
}

func (s *Scanner) scanAll(dirs []string, strict bool) (int, error) {
	offsets := s.loadOffsets()
	total := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if strict {
					return err
				}
				return nil // skip inaccessible
			}
			if d.IsDir() || !s.match(path) {
				return nil
			}
			// A full walk mostly rediscovers unchanged files. When the offsets
			// were preloaded, skip the open entirely for files whose size has
			// not moved since the last scan.
			if offsets != nil {
				if info, err := d.Info(); err == nil {
					if offset, ok := offsets[path]; ok && info.Size() == offset {
						return nil
					}
				}
			}
			n, err := s.scanFile(path, strict, offsets)
			if err != nil {
				if strict {
					return err
				}
				return nil // skip broken files, don't abort scan
			}
			total += n
			return nil
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ScanKnownFiles reads known file paths from the store and directly stats them.
// This is extremely fast compared to walking directories. When dirs is
// non-empty, only files under those dirs are considered.
func (s *Scanner) ScanKnownFiles(dirs []string) (int, error) {
	offsets := s.loadOffsets()
	var files []string
	if offsets != nil {
		// scan_offsets is the same table KnownFiles reads, so the preloaded
		// map already holds every known path.
		files = make([]string, 0, len(offsets))
		for path := range offsets {
			files = append(files, path)
		}
	} else {
		known, err := s.sink.KnownFiles()
		if err != nil {
			return 0, err
		}
		files = known
	}

	total := 0
	for _, path := range files {
		if !s.match(path) || !underDirs(path, dirs) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue // skip broken files
		}
		offset, err := s.offsetFor(offsets, path)
		if err != nil {
			continue
		}
		if info.Size() == offset {
			continue
		}
		n, _ := s.scanFile(path, false, offsets)
		total += n
	}
	return total, nil
}

// ScanFile incrementally scans one file. It is called for filesystem events.
func (s *Scanner) ScanFile(path string) (int, error) {
	if !s.match(path) {
		return 0, nil
	}
	return s.scanFile(path, false, nil)
}

func underDirs(path string, dirs []string) bool {
	if len(dirs) == 0 {
		return false
	}
	clean := filepath.Clean(path)
	for _, dir := range dirs {
		rel, err := filepath.Rel(filepath.Clean(dir), clean)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}

func (s *Scanner) scanFile(path string, processPartialEOF bool, offsets map[string]int64) (int, error) {
	offset, err := s.offsetFor(offsets, path)
	if err != nil {
		return 0, err
	}

	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, err
	}

	if info.Size() < offset {
		offset = 0 // file was truncated/rotated, rescan from start
	}
	if info.Size() == offset {
		return 0, nil // no new data
	}

	if _, err := file.Seek(offset, 0); err != nil {
		return 0, err
	}

	// Collect all entries, dedup by UUID (keep last = final token counts).
	uuidMap := make(map[string]scanEntry) // uuid -> last entry
	var noUUID []scanEntry                // entries without uuid

	if fileParser, ok := s.parser.(source.FileLineParser); ok {
		fileParser.ResetFile(path, offset)
	}
	currentProject := s.parser.ProjectFromPath(path)

	reader := bufio.NewReader(file)
	newOffset := offset

	for {
		lineBytes, err := reader.ReadBytes('\n')
		completeLine := len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n'
		partialEOF := processPartialEOF && len(lineBytes) > 0 && err == io.EOF
		if completeLine || partialEOF {
			lineStartOffset := newOffset
			line := string(lineBytes)
			u, projectHint, ok := s.parser.Parse(line)
			if projectHint != "" {
				currentProject = projectHint
			}
			if ok {
				newOffset += int64(len(lineBytes))
				u = watcher.WithProjectFallback(u, currentProject)
				if u.UUID == "" {
					u.UUID = s.parser.FallbackUUID(s.sink.DeviceID(), path, lineStartOffset, u)
				}
				ts := u.Timestamp
				if ts.IsZero() {
					ts = info.ModTime()
				}
				e := scanEntry{u: u, ts: ts}
				if u.UUID != "" {
					uuidMap[u.UUID] = e // overwrite = keep last
				} else {
					noUUID = append(noUUID, e)
				}
			} else if completeLine {
				newOffset += int64(len(lineBytes))
			}
		} else {
			// Partial line at EOF, don't advance offset, just break and wait for more.
			break
		}
		if err != nil {
			break
		}
	}

	entries := make([]scanEntry, 0, len(uuidMap)+len(noUUID))
	for _, e := range uuidMap {
		entries = append(entries, e)
	}
	entries = append(entries, noUUID...)
	count, err := s.emitEntries(path, entries, newOffset)
	if err == nil && offsets != nil {
		offsets[path] = newOffset
	}
	return count, err
}

// emitEntries writes a file's records, its directory marker, and its new
// offset as one unit. Sinks that support transactions commit them together so
// a file costs a single fsync instead of one per statement.
func (s *Scanner) emitEntries(path string, entries []scanEntry, newOffset int64) (int, error) {
	write := func(sink source.JSONLStore) (int, error) {
		count := 0
		for _, e := range entries {
			e.u.Timestamp = e.ts
			if err := sink.Emit(e.u, path); err != nil {
				return count, err
			}
			count++
		}
		// Cache this directory for fast startup next time.
		if err := sink.UpsertKnownDir(filepath.Dir(path)); err != nil {
			return count, err
		}
		// Update the offset only after inserts succeed.
		if err := sink.SetOffset(path, newOffset); err != nil {
			return count, err
		}
		return count, nil
	}

	runner, ok := s.sink.(source.TxRunner)
	if !ok {
		return write(s.sink)
	}

	var count int
	err := runner.RunInTx(func(txSink source.Sink) error {
		store, ok := txSink.(source.JSONLStore)
		if !ok {
			return fmt.Errorf("scanner: transaction sink does not implement JSONLStore")
		}
		var err error
		count, err = write(store)
		return err
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// loadOffsets preloads every recorded offset in one query when the sink
// supports it, avoiding an N+1 pattern over known files.
func (s *Scanner) loadOffsets() map[string]int64 {
	index, ok := s.sink.(source.OffsetIndex)
	if !ok {
		return nil
	}
	offsets, err := index.AllOffsets()
	if err != nil {
		return nil
	}
	return offsets
}

func (s *Scanner) offsetFor(offsets map[string]int64, path string) (int64, error) {
	if offsets != nil {
		return offsets[path], nil
	}
	return s.sink.Offset(path)
}

// defaultParser preserves the original Claude/Gemini behavior for tests and
// for callers that do not need a source-specific filter.
type defaultParser struct{}

func (defaultParser) Parse(line string) (model.TokenUsage, string, bool) {
	project := watcher.ExtractProjectNameFromLine(line)
	u, ok := watcher.ParseLine(line)
	return u, project, ok
}

func (defaultParser) ProjectFromPath(path string) string {
	return watcher.ExtractProjectName(path)
}

func (defaultParser) FallbackUUID(deviceID, path string, lineStartOffset int64, u model.TokenUsage) string {
	if u.Source == "Gemini CLI" {
		return watcher.StableGeminiUUID(deviceID, path, lineStartOffset)
	}
	return ""
}
