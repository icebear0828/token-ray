package sources

import (
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

// NewPiSource watches Pi Agent session transcripts.
func NewPiSource(home string) *JSONLSource {
	return newJSONLSource(
		source.Descriptor{ID: "pi-agent", Source: piSourceName, Label: "PI", Builtin: true},
		[]string{piSessionsDir(home)},
		&piParser{},
	)
}

func piSessionsDir(home string) string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return filepath.Join(dir, "sessions")
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

// piParser keeps the model and session id learned from earlier lines in the
// same transcript. The scanner serializes scans per source, but the mutex
// keeps the parser safe if a future caller bypasses the runner.
type piParser struct {
	mu           sync.Mutex
	currentModel string
	sessionID    string
}

func (p *piParser) ResetFile(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentModel = ""
	p.sessionID = piSessionIDFromPath(path)
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
		if record.ID != "" {
			p.sessionID = record.ID
		}
		if record.ModelID != "" {
			p.currentModel = record.ModelID
		} else if record.Model != "" {
			p.currentModel = record.Model
		}
		project := ""
		if record.Cwd != "" {
			project = watcher.ExtractProjectNameFromCWD(record.Cwd)
		}
		return model.TokenUsage{}, project, false
	case "model_change":
		if record.ModelID != "" {
			p.currentModel = record.ModelID
		} else if record.Model != "" {
			p.currentModel = record.Model
		}
		return model.TokenUsage{}, "", false
	}

	var usage *piUsage
	switch {
	case record.Type == "message" && (record.Message.Role == "assistant" || record.Message.Role == "toolResult"):
		usage = record.Message.Usage
		if record.Message.Model != "" {
			p.currentModel = record.Message.Model
		}
	case record.Type == "compaction" || record.Type == "branch_summary" || record.Type == "usage":
		usage = record.Usage
	}
	if usage == nil {
		return model.TokenUsage{}, "", false
	}

	u := piTokenUsage(usage, p.currentModel, p.sessionID, record)
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CachedTokens == 0 && u.CacheCreationTokens == 0 {
		return model.TokenUsage{}, "", false
	}
	return u, "", true
}

func (p *piParser) ProjectFromPath(string) string { return "" }

func (p *piParser) FallbackUUID(string, string, int64, model.TokenUsage) string { return "" }

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
