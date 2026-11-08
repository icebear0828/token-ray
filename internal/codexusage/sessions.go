package codexusage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

type codexSessionLine struct {
	Timestamp string              `json:"timestamp"`
	Type      string              `json:"type"`
	Payload   codexSessionPayload `json:"payload"`
}

type codexSessionPayload struct {
	ID    string            `json:"id"`
	CWD   string            `json:"cwd"`
	Model string            `json:"model"`
	Type  string            `json:"type"`
	Info  *codexSessionInfo `json:"info"`
}

type codexSessionInfo struct {
	TotalTokenUsage codexSessionTokenUsage `json:"total_token_usage"`
}

type codexSessionTokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

type codexSessionTotal struct {
	id        string
	cwd       string
	model     string
	ts        time.Time
	tokens    codexSessionTokenUsage
	hasTokens bool
}

func (s *Scanner) scanSessions(threads map[string]threadInfo) (int, bool, error) {
	if s.SessionsDir == "" {
		return 0, false, nil
	}
	if _, err := os.Stat(s.SessionsDir); err != nil {
		return 0, false, nil
	}

	count := 0
	hasSessionTotals := false
	err := filepath.WalkDir(s.SessionsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		n, hasTotals, err := s.scanSessionFile(path, threads)
		if err != nil {
			return err
		}
		count += n
		if hasTotals {
			hasSessionTotals = true
		}
		return nil
	})
	if err != nil {
		return count, hasSessionTotals, err
	}
	return count, hasSessionTotals, nil
}

func (s *Scanner) scanSessionFile(path string, threads map[string]threadInfo) (int, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false, nil
	}
	offsetKey := "codex:session:" + filepath.ToSlash(path)
	offset, err := s.sink.Offset(offsetKey)
	if err != nil {
		return 0, false, err
	}
	if info.Size() == offset && offset > 0 {
		return 0, true, nil
	}

	total, err := parseCodexSessionFile(path, info.ModTime())
	if err != nil || !total.hasTokens {
		return 0, false, err
	}
	if thread, ok := threads[total.id]; ok {
		if total.model == "" {
			total.model = thread.model
		}
		if total.cwd == "" {
			total.cwd = thread.cwd
		}
	}
	if total.model == "" {
		total.model = "codex"
	}
	if total.id == "" {
		total.id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	project := watcher.ExtractProjectNameFromCWD(total.cwd)
	if project == "" {
		project = "Default"
	}
	usage := model.TokenUsage{
		Source:       SourceName,
		Model:        total.model,
		Project:      project,
		InputTokens:  total.tokens.InputTokens,
		CachedTokens: total.tokens.CachedInputTokens,
		OutputTokens: total.tokens.OutputTokens,
		Thoughts:     total.tokens.ReasoningOutputTokens,
		Timestamp:    total.ts,
		UUID:         "codex-session:" + total.id,
	}
	if err := s.sink.Emit(usage, path); err != nil {
		return 0, true, err
	}
	if err := s.sink.SetOffset(offsetKey, info.Size()); err != nil {
		return 0, true, err
	}
	return 1, true, nil
}

func parseCodexSessionFile(path string, fallbackTime time.Time) (codexSessionTotal, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexSessionTotal{}, err
	}
	defer file.Close()

	total := codexSessionTotal{ts: fallbackTime.UTC()}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		var line codexSessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch line.Type {
		case "session_meta":
			if line.Payload.ID != "" {
				total.id = line.Payload.ID
			}
			if line.Payload.CWD != "" {
				total.cwd = line.Payload.CWD
			}
			if line.Payload.Model != "" {
				total.model = line.Payload.Model
			}
		case "event_msg":
			if line.Payload.Type != "token_count" || line.Payload.Info == nil {
				continue
			}
			tokens := line.Payload.Info.TotalTokenUsage
			if tokens.InputTokens == 0 && tokens.CachedInputTokens == 0 && tokens.OutputTokens == 0 {
				continue
			}
			total.tokens = tokens
			total.hasTokens = true
			if parsed, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				total.ts = parsed.UTC()
			}
		}
	}
	return total, scanner.Err()
}
