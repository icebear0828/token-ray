package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"ai-flight-dashboard/internal/model"

	"github.com/fsnotify/fsnotify"
)

// TokenUsage is an alias for model.TokenUsage.
// Retained for backward compatibility with existing consumers.
type TokenUsage = model.TokenUsage

type Watcher struct {
	fw            *fsnotify.Watcher
	offsets       map[string]int64
	mu            sync.Mutex
	UsageChan     chan TokenUsage
	BroadcastChan chan TokenUsage // For LAN broadcasting of real-time events
	done          chan struct{}
	recursiveDirs map[string]bool // tracks dirs registered for recursive watching
	DeviceID      string
	paused        atomic.Bool
	handlerMu     sync.RWMutex
	fileHandler   func(string)
}

func New(deviceID string) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fw:            fw,
		offsets:       make(map[string]int64),
		UsageChan:     make(chan TokenUsage, 100),
		BroadcastChan: make(chan TokenUsage, 100),
		done:          make(chan struct{}),
		recursiveDirs: make(map[string]bool),
		DeviceID:      deviceID,
	}

	go w.listen()
	return w, nil
}

func (w *Watcher) IsPaused() bool {
	return w.paused.Load()
}

func (w *Watcher) SetPaused(p bool) {
	w.paused.Store(p)
}

// SetFileHandler replaces the built-in JSONL parser with a callback. The
// dashboard uses this to route filesystem events to registered sources.
func (w *Watcher) SetFileHandler(fn func(string)) {
	w.handlerMu.Lock()
	w.fileHandler = fn
	w.handlerMu.Unlock()
}

func (w *Watcher) handleFile(path string) bool {
	w.handlerMu.RLock()
	fn := w.fileHandler
	w.handlerMu.RUnlock()
	if fn == nil {
		return false
	}
	fn(path)
	return true
}

func (w *Watcher) WatchDir(dir string) error {
	return w.fw.Add(dir)
}

// WatchDirRecursive walks all existing subdirs and watches them.
// New subdirs created later are auto-registered in listen().
func (w *Watcher) WatchDirRecursive(dir string) error {
	w.mu.Lock()
	w.recursiveDirs[dir] = true
	w.mu.Unlock()

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return w.fw.Add(path)
		}
		return nil
	})
}

// UnwatchDir removes a directory and all its existing subdirectories from the watcher.
func (w *Watcher) UnwatchDir(dir string) error {
	w.mu.Lock()
	delete(w.recursiveDirs, dir)
	w.mu.Unlock()

	if err := w.fw.Remove(dir); err != nil {
		log.Printf("watcher: failed to unwatch %s: %v", dir, err)
	}

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if rmErr := w.fw.Remove(path); rmErr != nil {
				log.Printf("watcher: failed to unwatch %s: %v", path, rmErr)
			}
		}
		return nil
	})
}

// WatchKnownDirs registers specific directories without recursive walk.
// Used with cached directory lists for fast startup.
func (w *Watcher) WatchKnownDirs(dirs []string) {
	for _, dir := range dirs {
		w.fw.Add(dir)
	}
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.fw.Close()
}

func (w *Watcher) listen() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if w.isUnderRecursiveRoot(event.Name) {
						w.fw.Add(event.Name)
					}
				}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.processFile(event.Name)
			}
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Println("watcher error:", err)
		}
	}
}

// isUnderRecursiveRoot checks if path falls under any recursiveDirs root.
func (w *Watcher) isUnderRecursiveRoot(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for root := range w.recursiveDirs {
		if strings.HasPrefix(path, root) {
			return true
		}
	}
	return false
}
