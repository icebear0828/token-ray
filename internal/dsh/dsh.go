package dsh

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-flight-dashboard/internal/source"
)

const sourceName = "DeepSeek Harness"

var transcriptNamePattern = regexp.MustCompile(`^session(?:\.v(\d+))?\.jsonl(\.zstd)?$`)

// Source watches DeepSeek Harness session transcripts.
type Source struct {
	root string
	mu   sync.Mutex
}

func New(home string) *Source {
	root := strings.TrimSpace(os.Getenv("DSH_HOME"))
	if root == "" {
		root = filepath.Join(home, ".dsh")
	}
	return &Source{root: root}
}

func (s *Source) Descriptor() source.Descriptor {
	return source.Descriptor{ID: "dsh", Source: sourceName, Label: "DSH", Builtin: true}
}

func (s *Source) WatchDirs() []string {
	dir := filepath.Join(s.root, "sessions")
	if _, err := os.Stat(dir); err == nil {
		return []string{dir}
	}
	return nil
}

func (s *Source) Scan(ctx context.Context, sink source.Sink) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dirs, err := sessionDirs(filepath.Join(s.root, "sessions"))
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.scanSessionDir(dir, sink); err != nil {
			return err
		}
	}
	return nil
}

func (s *Source) ScanFile(ctx context.Context, path string, sink source.Sink) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	dir := path
	if !info.IsDir() {
		if !isTranscriptName(filepath.Base(path)) {
			return nil
		}
		dir = filepath.Dir(path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.scanSessionDir(dir, sink)
}

func (s *Source) scanSessionDir(dir string, sink source.Sink) error {
	best, ok := preferredTranscript(dir)
	if !ok {
		return nil
	}
	path := filepath.Join(dir, best.name)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	offset, err := sink.Offset(path)
	if err != nil {
		return err
	}
	if info.Size() == offset {
		return nil
	}

	usage, err := parseTranscriptFile(path, best.compressed, filepath.Base(dir))
	if err != nil {
		return err
	}
	for _, u := range usage {
		if err := sink.Emit(u, path); err != nil {
			return err
		}
	}
	return sink.SetOffset(path, info.Size())
}

type transcript struct {
	name       string
	version    int
	compressed bool
	modTime    time.Time
}

func sessionDirs(root string) ([]string, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := make(map[string]struct{})
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isTranscriptName(entry.Name()) {
			return nil
		}
		dir := filepath.Dir(path)
		if _, ok := seen[dir]; ok {
			return nil
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

func preferredTranscript(dir string) (transcript, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return transcript{}, false
	}
	candidates := make([]transcript, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		candidate, ok := parseTranscriptName(entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidate.modTime = info.ModTime()
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return transcript{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].version != candidates[j].version {
			return candidates[i].version > candidates[j].version
		}
		if !candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].modTime.After(candidates[j].modTime)
		}
		if candidates[i].compressed != candidates[j].compressed {
			return candidates[i].compressed
		}
		return candidates[i].name < candidates[j].name
	})
	return candidates[0], true
}

func parseTranscriptName(name string) (transcript, bool) {
	match := transcriptNamePattern.FindStringSubmatch(name)
	if match == nil {
		return transcript{}, false
	}
	version := 0
	if match[1] != "" {
		version, _ = strconv.Atoi(match[1])
	}
	return transcript{name: name, version: version, compressed: match[2] != ""}, true
}

func isTranscriptName(name string) bool {
	_, ok := parseTranscriptName(name)
	return ok
}
