package agyscanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"path/filepath"
	"time"

	"ai-flight-dashboard/internal/antigravity"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"
)

type Scanner struct {
	db       *db.DB
	calc     *calculator.Calculator
	DeviceID string
	LogDir   string
	HistPath string
}

func New(database *db.DB, calc *calculator.Calculator, deviceID string) *Scanner {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".gemini", "antigravity-cli")
	return &Scanner{
		db:       database,
		calc:     calc,
		DeviceID: deviceID,
		LogDir:   filepath.Join(base, "log"),
		HistPath: filepath.Join(base, "history.jsonl"),
	}
}

func (s *Scanner) Scan(usageChan chan<- watcher.TokenUsage) (int, error) {
	ports := discoverPorts(s.LogDir)
	if len(ports) == 0 {
		return 0, nil
	}

	workspaces := s.loadWorkspaces()
	total := 0

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

