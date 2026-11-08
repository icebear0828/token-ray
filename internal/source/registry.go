package source

import (
	"path/filepath"
	"strings"
	"sync"
)

// Registry holds the active sources. It is safe for concurrent use because
// plugins can be installed or disabled while the scan loop is running.
type Registry struct {
	mu      sync.RWMutex
	sources []Source
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(s Source) {
	if s == nil {
		return
	}
	id := s.Descriptor().ID
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, existing := range r.sources {
		if existing.Descriptor().ID == id {
			r.sources[i] = s
			return
		}
	}
	r.sources = append(r.sources, s)
}

func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.sources {
		if s.Descriptor().ID == id {
			r.sources = append(r.sources[:i], r.sources[i+1:]...)
			return
		}
	}
}

func (r *Registry) All() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Source, len(r.sources))
	copy(out, r.sources)
	return out
}

func (r *Registry) Get(id string) Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.sources {
		if s.Descriptor().ID == id {
			return s
		}
	}
	return nil
}

// Descriptors returns visible sources in registration order so the UI tab
// order stays stable.
func (r *Registry) Descriptors() []Descriptor {
	all := r.All()
	out := make([]Descriptor, 0, len(all))
	for _, s := range all {
		d := s.Descriptor()
		if d.Hidden {
			continue
		}
		out = append(out, d)
	}
	return out
}

// SourceForPath returns the source whose watch directory contains path.
// Longest-prefix matching keeps overlapping directories predictable.
func (r *Registry) SourceForPath(path string) Source {
	clean := filepath.Clean(path)
	all := r.All()
	var best Source
	bestLen := -1
	for _, s := range all {
		for _, dir := range s.WatchDirs() {
			rel, err := filepath.Rel(filepath.Clean(dir), clean)
			if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if len(dir) > bestLen {
				best = s
				bestLen = len(dir)
			}
		}
	}
	return best
}
