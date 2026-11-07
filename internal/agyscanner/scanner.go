package agyscanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ai-flight-dashboard/internal/antigravity"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"
)

type Scanner struct {
	db           *db.DB
	calc         *calculator.Calculator
	DeviceID     string
	LogDir       string
	HistPath     string
	BrainDir     string
	SettingsPath string
}

func New(database *db.DB, calc *calculator.Calculator, deviceID string) *Scanner {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".gemini", "antigravity-cli")
	return &Scanner{
		db:           database,
		calc:         calc,
		DeviceID:     deviceID,
		LogDir:       filepath.Join(base, "log"),
		HistPath:     filepath.Join(base, "history.jsonl"),
		BrainDir:     filepath.Join(base, "brain"),
		SettingsPath: filepath.Join(base, "settings.json"),
	}
}

func (s *Scanner) Scan(usageChan chan<- watcher.TokenUsage) (int, error) {
	total := 0
	if n, err := s.ScanTranscripts(usageChan); err == nil {
		total += n
	}

	ports := discoverPorts(s.LogDir)
	if len(ports) == 0 {
		return total, nil
	}

	workspaces := s.loadWorkspaces()
	for _, ap := range ports {
		summaries, err := getAllTrajectories(ap.Port)
		if err != nil {
			continue
		}
		for cascadeID := range summaries {
			n, err := s.scanTrajectory(ap.Port, cascadeID, workspaces, usageChan)
			if err != nil {
				continue
			}
			total += n
		}
	}
	return total, nil
}

func (s *Scanner) scanTrajectory(port int, cascadeID string, workspaces map[string]string, usageChan chan<- watcher.TokenUsage) (int, error) {
	offsetKey := "agy:" + cascadeID + ":gmCount"
	lastCount, _ := s.db.GetOffset(offsetKey)

	resp, err := getTrajectory(port, cascadeID)
	if err != nil {
		return 0, err
	}

	gm := resp.Trajectory.GeneratorMetadata
	if int64(len(gm)) <= lastCount {
		return 0, nil
	}

	project := watcher.ExtractProjectNameFromCWD(workspaces[cascadeID])
	count := 0

	for i := int(lastCount); i < len(gm); i++ {
		cm := gm[i].ChatModel
		usage := cm.Usage
		input := usage.InputInt()
		output := usage.OutputInt()
		if input == 0 && output == 0 {
			continue
		}

		cached := usage.CacheReadInt()
		totalInput := input + cached
		modelName := normalizeDisplayName(cm.ModelDisplayName)
		if modelName == "" {
			modelName = normalizeResponseModel(cm.ResponseModel)
		}
		if modelName == "" {
			modelName = "unknown"
		}

		ts := parseTimestamp(cm.ChatStartMetadata.CreatedAt)
		uuid := fmt.Sprintf("agy:%s:%d", cascadeID, i)

		cost, _ := s.calc.CalculateCost(modelName, totalInput, cached, 0, output)

		u := watcher.TokenUsage{
			Source:       antigravity.Source,
			Model:        modelName,
			Project:      project,
			InputTokens:  totalInput,
			CachedTokens: cached,
			OutputTokens: output,
			Timestamp:   ts,
			UUID:        uuid,
		}
		if usageChan != nil {
			usageChan <- u
		}
		if err := s.db.InsertUsageWithTime(u, cost, ts, "agy://"+cascadeID, s.DeviceID); err != nil {
			continue
		}
		count++
	}

	if count > 0 || int64(len(gm)) > lastCount {
		s.db.SetOffset(offsetKey, int64(len(gm)))
	}
	return count, nil
}

func (s *Scanner) loadWorkspaces() map[string]string {
	f, err := os.Open(s.HistPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	ws := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var entry struct {
			ConversationID string `json:"conversationId"`
			Workspace      string `json:"workspace"`
		}
		if json.Unmarshal(sc.Bytes(), &entry) == nil && entry.ConversationID != "" && entry.Workspace != "" {
			ws[entry.ConversationID] = entry.Workspace
		}
	}
	return ws
}

// normalizeDisplayName converts display names to pricing table keys:
//   "Gemini 3.5 Flash (Low)" → "gemini-3.5-flash"
//   "Claude Sonnet 4.6 (Thinking)" → "claude-sonnet-4-6"
func normalizeDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if idx := strings.LastIndex(name, "("); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	lower := strings.ToLower(name)
	// Gemini keeps dots in version: "gemini-3.5-flash"
	keepDot := strings.HasPrefix(lower, "gemini")
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r == '.' && keepDot) {
			b.WriteRune(r)
			lastDash = false
		} else if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// normalizeResponseModel strips internal suffixes (e.g. "-a", "-b") from
// Google's internal model IDs so they match the pricing table entries.
func normalizeResponseModel(name string) string {
	if name == "" {
		return ""
	}
	for _, suffix := range []string{"-thinking", "-extra-low", "-high", "-medium", "-low", "-a", "-b", "-c", "-d"} {
		if strings.HasSuffix(name, suffix) {
			name = name[:len(name)-len(suffix)]
		}
	}
	if name == "gemini-default" {
		return ""
	}
	return name
}

func parseTimestamp(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// ScanWithPort runs a scan against a specific port (for testing).
func (s *Scanner) ScanWithPort(port int, usageChan chan<- watcher.TokenUsage) (int, error) {
	workspaces := s.loadWorkspaces()
	summaries, err := getAllTrajectories(port)
	if err != nil {
		return 0, err
	}

	total := 0
	for cascadeID := range summaries {
		n, err := s.scanTrajectory(port, cascadeID, workspaces, usageChan)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
}

var modelChangeRe = regexp.MustCompile(`Model Selection` + "`" + ` from (?:None|[^\s` + "`" + `]+) to ([^` + "`" + `\r\n]+)`)

type transcriptEntry struct {
	StepIndex       int    `json:"step_index"`
	Type            string `json:"type"`
	CreatedAt       string `json:"created_at"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	CacheReadTokens int    `json:"cache_read_tokens"`
	Content         string `json:"content"`
}

func (s *Scanner) loadDefaultModel() string {
	if s.SettingsPath == "" {
		return "gemini-3.8-flash"
	}
	data, err := os.ReadFile(s.SettingsPath)
	if err != nil {
		return "gemini-3.8-flash"
	}
	var settings struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &settings); err == nil && settings.Model != "" {
		normalized := normalizeDisplayName(settings.Model)
		if normalized != "" {
			return normalized
		}
	}
	return "gemini-3.8-flash"
}

func (s *Scanner) ScanTranscripts(usageChan chan<- watcher.TokenUsage) (int, error) {
	if s.BrainDir == "" {
		return 0, nil
	}
	if _, err := os.Stat(s.BrainDir); os.IsNotExist(err) {
		return 0, nil
	}

	workspaces := s.loadWorkspaces()
	defaultModel := s.loadDefaultModel()

	entries, err := os.ReadDir(s.BrainDir)
	if err != nil {
		return 0, err
	}

	total := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cascadeID := e.Name()
		transcriptPath := filepath.Join(s.BrainDir, cascadeID, ".system_generated", "logs", "transcript.jsonl")
		info, err := os.Stat(transcriptPath)
		if err != nil || info.Size() == 0 {
			continue
		}

		offsetKey := "agy:transcript:" + cascadeID
		lastOffset, _ := s.db.GetOffset(offsetKey)
		if info.Size() <= lastOffset {
			continue
		}

		n, newOffset, err := s.scanTranscriptFile(transcriptPath, cascadeID, lastOffset, defaultModel, workspaces, usageChan)
		if err != nil {
			continue
		}
		if newOffset > lastOffset {
			s.db.SetOffset(offsetKey, newOffset)
		}
		total += n
	}
	return total, nil
}

func (s *Scanner) scanTranscriptFile(path, cascadeID string, fromOffset int64, defaultModel string, workspaces map[string]string, usageChan chan<- watcher.TokenUsage) (int, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fromOffset, err
	}
	defer f.Close()

	if fromOffset > 0 {
		if _, err := f.Seek(fromOffset, io.SeekStart); err != nil {
			return 0, fromOffset, err
		}
	}

	project := watcher.ExtractProjectNameFromCWD(workspaces[cascadeID])
	currentModel := defaultModel
	reader := bufio.NewReader(f)
	currentOffset := fromOffset
	inserted := 0

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			lineLen := int64(len(line))
			var entry transcriptEntry
			if err := json.Unmarshal(line, &entry); err == nil {
				if entry.Type == "USER_INPUT" && entry.Content != "" {
					if m := modelChangeRe.FindStringSubmatch(entry.Content); len(m) > 1 {
						if norm := normalizeDisplayName(m[1]); norm != "" {
							currentModel = norm
						}
					}
				} else if entry.Type == "PLANNER_RESPONSE" && (entry.InputTokens > 0 || entry.OutputTokens > 0) {
					input := entry.InputTokens
					cached := entry.CacheReadTokens
					totalInput := input + cached
					output := entry.OutputTokens
					ts := parseTimestamp(entry.CreatedAt)
					uuid := fmt.Sprintf("agy:%s:%d", cascadeID, entry.StepIndex)

					cost, _ := s.calc.CalculateCost(currentModel, totalInput, cached, 0, output)
					u := watcher.TokenUsage{
						Source:       antigravity.Source,
						Model:        currentModel,
						Project:      project,
						InputTokens:  totalInput,
						CachedTokens: cached,
						OutputTokens: output,
						Timestamp:    ts,
						UUID:         uuid,
					}
					if usageChan != nil {
						usageChan <- u
					}
					if err := s.db.InsertUsageWithTime(u, cost, ts, path, s.DeviceID); err == nil {
						inserted++
					}
				}
			}
			currentOffset += lineLen
		}
		if readErr != nil {
			break
		}
	}
	return inserted, currentOffset, nil
}

