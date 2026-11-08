package watcher

import (
	"bufio"
	"os"
	"strings"
)

// processFile reads the new lines of the file.
func (w *Watcher) processFile(path string) {
	if w.handleFile(path) {
		return
	}
	if !strings.HasSuffix(path, ".jsonl") {
		return
	}

	w.mu.Lock()
	offset := w.offsets[path]
	w.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return
	}

	if info.Size() < offset {
		offset = 0
	}

	_, err = file.Seek(offset, 0)
	if err != nil {
		return
	}

	projectName := ExtractProjectName(path)
	currentProject := projectName

	reader := bufio.NewReader(file)
	newOffset := offset

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
			lineStartOffset := newOffset
			newOffset += int64(len(lineBytes))
			line := string(lineBytes)
			if project := ExtractProjectNameFromLine(line); project != "" {
				currentProject = project
			}
			if w.IsPaused() {
				continue
			}
			if u, ok := ParseLine(line); ok {
				u = WithProjectFallback(u, currentProject)
				if u.Source == "Gemini CLI" && u.UUID == "" {
					u.UUID = StableGeminiUUID(w.DeviceID, path, lineStartOffset)
				}
				w.UsageChan <- u
				select {
				case w.BroadcastChan <- u:
				default:
				}
			}
		} else {
			break
		}
		if err != nil {
			break
		}
	}

	w.mu.Lock()
	w.offsets[path] = newOffset
	w.mu.Unlock()
}
