package plugin

import (
	"context"
	"os"
	"time"

	"ai-flight-dashboard/internal/scanner"
	"ai-flight-dashboard/internal/source"
)

// Source adapts a declarative plugin manifest to the source.Source contract.
type Source struct {
	manifest Manifest
	dirs     []string
	runner   *scanner.Runner
}

func NewSource(m Manifest) (*Source, error) {
	dirs, err := m.ExpandWatchDirs()
	if err != nil {
		return nil, err
	}
	dirsFn := func() []string { return existingDirs(dirs) }
	return &Source{
		manifest: m,
		dirs:     dirs,
		runner: &scanner.Runner{
			Dirs:         dirsFn,
			Parser:       &parser{manifest: m},
			Match:        m.Matcher(),
			FullInterval: time.Minute,
		},
	}, nil
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

func (s *Source) Descriptor() source.Descriptor {
	return source.Descriptor{
		ID:          s.manifest.ID,
		Source:      s.manifest.Source,
		Label:       s.manifest.LabelText(),
		Description: s.manifest.Description,
		Version:     s.manifest.Version,
	}
}

func (s *Source) WatchDirs() []string { return existingDirs(s.dirs) }

func (s *Source) Scan(ctx context.Context, sink source.Sink) error {
	store, ok := sink.(source.JSONLStore)
	if !ok {
		return nil
	}
	return s.runner.Scan(ctx, store)
}

func (s *Source) ScanFile(ctx context.Context, path string, sink source.Sink) error {
	store, ok := sink.(source.JSONLStore)
	if !ok {
		return nil
	}
	return s.runner.ScanFile(ctx, path, store)
}
