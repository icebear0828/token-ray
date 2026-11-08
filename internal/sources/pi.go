package sources

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/watcher"
)

const piSourceName = "Pi Agent"

type piUsage struct {
	Input           int `json:"input"`
	Output          int `json:"output"`
	CacheRead       int `json:"cacheRead"`
	CacheWrite      int `json:"cacheWrite"`
	Reasoning       int `json:"reasoning"`
	ReasoningTokens int `json:"reasoningTokens"`
}

type piRecord struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Timestamp string   `json:"timestamp"`
	Cwd       string   `json:"cwd"`
	Provider  string   `json:"provider"`
	Model     string   `json:"model"`
	ModelID   string   `json:"modelId"`
	Usage     *piUsage `json:"usage"`
	Message   struct {
		Role     string   `json:"role"`
		Model    string   `json:"model"`
		Provider string   `json:"provider"`
		Usage    *piUsage `json:"usage"`
	} `json:"message"`
}

type piFileState struct {
	sessionID string
	model     string
	project   string
}

// NewPiSource watches Pi Agent session transcripts.
func NewPiSource(home string) *JSONLSource {
	return newJSONLSource(
		source.Descriptor{ID: "pi-agent", Source: piSourceName, Label: "PI", Builtin: true},
		[]string{piSessionsDir(home)},
		newPiParser(),
	)
}

func piSessionsDir(home string) string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return filepath.Join(dir, "sessions")
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

// piParser keeps the model, session id and project learned from earlier lines
// in the same transcript. The scanner serializes scans per source, but the
// mutex keeps the parser safe if a future caller bypasses the runner.
type piParser struct {
	mu          sync.Mutex
	states      map[string]piFileState
	currentPath string
	current     piFileState
}

func newPiParser() *piParser {
	return &piParser{states: make(map[string]piFileState)}
}

func (p *piParser) ResetFile(path string, offset int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.states == nil {
		p.states = make(map[string]piFileState)
	}
	p.currentPath = path
	if offset == 0 {
		p.current = piFileState{
			sessionID: piSessionIDFromPath(path),
			project:   piProjectFromDir(path),
		}
		p.states[path] = p.current
		return
	}
	if state, ok := p.states[path]; ok {
		p.current = state
		return
	}
	state := piFileState{
		sessionID: piSessionIDFromPath(path),
		project:   piProjectFromDir(path),
	}
	state = p.readPrefixState(path, offset, state)
	p.states[path] = state
	p.current = state
}

// readPrefixState recovers the model/session/project state after a process
// restart. The session header and model_change records are written before the
// stored offset, so replaying only that prefix is enough to resume an
// incremental scan mid-file.
func (p *piParser) readPrefixState(path string, limit int64, state piFileState) piFileState {
	file, err := os.Open(path)
	if err != nil {
		return state
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var read int64
	for read < limit {
		line, err := reader.ReadString('\n')
		read += int64(len(line))
		if len(line) > 0 {
			state = applyPiLine(state, line)
		}
		if err != nil {
			break
		}
	}
	return state
}

func (p *piParser) Parse(line string) (model.TokenUsage, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var record piRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return model.TokenUsage{}, "", false
	}

	switch record.Type {
	case "session":
		p.current = p.current.applyRecord(record)
		p.cacheCurrent()
		return model.TokenUsage{}, p.current.project, false
	case "model_change":
		p.current = p.current.applyRecord(record)
		p.cacheCurrent()
		return model.TokenUsage{}, "", false
	}

	var usage *piUsage
	switch {
	case record.Type == "message" && (record.Message.Role == "assistant" || record.Message.Role == "toolResult"):
		usage = record.Message.Usage
		if record.Message.Model != "" {
			p.current.model = record.Message.Model
			p.cacheCurrent()
		}
	case record.Type == "compaction" || record.Type == "branch_summary" || record.Type == "usage":
		usage = record.Usage
	}
	if usage == nil {
		return model.TokenUsage{}, "", false
	}

	u := piTokenUsage(usage, p.current.model, p.current.sessionID, record)
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CachedTokens == 0 && u.CacheCreationTokens == 0 {
		return model.TokenUsage{}, "", false
	}
	return u, "", true
}

func (p *piParser) ProjectFromPath(string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current.project
}

func (p *piParser) FallbackUUID(deviceID, path string, lineStartOffset int64, _ model.TokenUsage) string {
	h := sha256.New()
	fmt.Fprintf(h, "pi|%s|%s|%d", deviceID, path, lineStartOffset)
	return "pi:" + hex.EncodeToString(h.Sum(nil))
}

func (p *piParser) cacheCurrent() {
	if p.currentPath == "" {
		return
	}
	if p.states == nil {
		p.states = make(map[string]piFileState)
	}
	p.states[p.currentPath] = p.current
}

func (s piFileState) applyRecord(record piRecord) piFileState {
	switch record.Type {
	case "session":
		if record.ID != "" {
			s.sessionID = record.ID
		}
		if record.ModelID != "" {
			s.model = record.ModelID
		} else if record.Model != "" {
			s.model = record.Model
		}
		if record.Cwd != "" {
			s.project = watcher.ExtractProjectNameFromCWD(record.Cwd)
		}
	case "model_change":
		if record.ModelID != "" {
			s.model = record.ModelID
		} else if record.Model != "" {
			s.model = record.Model
		}
	case "message":
		if record.Message.Role == "assistant" && record.Message.Model != "" {
			s.model = record.Message.Model
		}
	}
	return s
}

func applyPiLine(state piFileState, line string) piFileState {
	var record piRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return state
	}
	return state.applyRecord(record)
}

func piTokenUsage(usage *piUsage, currentModel, sessionID string, record piRecord) model.TokenUsage {
	reasoning := usage.Reasoning
	if reasoning == 0 {
		reasoning = usage.ReasoningTokens
	}
	modelName := strings.TrimSpace(currentModel)
	if modelName == "" {
		modelName = "unknown"
	}
	u := model.TokenUsage{
		Source:              piSourceName,
		Model:               modelName,
		// Pi reports input exclusive of cache reads/writes; our schema stores
		// total input and subtracts the cache buckets when computing base input.
		InputTokens:         usage.Input + usage.CacheRead + usage.CacheWrite,
		CachedTokens:        usage.CacheRead,
		CacheCreationTokens: usage.CacheWrite,
		OutputTokens:        usage.Output,
		Thoughts:            reasoning,
		Timestamp:           parsePiTimestamp(record.Timestamp),
	}
	if record.ID != "" {
		u.UUID = fmt.Sprintf("pi-%s-%s", sessionID, record.ID)
	}
	return u
}

func piSessionIDFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if idx := strings.LastIndex(base, "_"); idx >= 0 && idx+1 < len(base) {
		return base[idx+1:]
	}
	return base
}

// piProjectFromDir is a best-effort fallback for sessions whose header has not
// been seen yet. Pi sanitizes the working directory into a folder name such as
// --Users--c--myproject--; the last segment is the project name.
func piProjectFromDir(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	parts := strings.FieldsFunc(dir, func(r rune) bool { return r == '-' })
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func parsePiTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}
