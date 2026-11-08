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
	"sync"
	"time"

	"ai-flight-dashboard/internal/antigravity"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/watcher"
)

const (
	transcriptOffsetPrefix = "agy:transcript:"
	defaultModelName       = "gemini-3.8-flash"
)

type Scanner struct {
	sink         source.Sink
	LogDir       string
	HistPath     string
	BrainDir     string
	SettingsPath string

	mu           sync.Mutex
	lastFull     time.Time
	fullInterval time.Duration

	metaMu        sync.Mutex
	workspaces    map[string]string
	histSig       fileSignature
	histValid     bool
	defaultModel  string
	settingsSig   fileSignature
	settingsValid bool
}

// fileSignature identifies a metadata file's on-disk state so it is only
// re-read after it changes.
type fileSignature struct {
	modTime time.Time
	size    int64
}

func New(sink source.Sink) *Scanner {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".gemini", "antigravity-cli")
	return &Scanner{
		sink:         sink,
		LogDir:       filepath.Join(base, "log"),
		HistPath:     filepath.Join(base, "history.jsonl"),
		BrainDir:     filepath.Join(base, "brain"),
		SettingsPath: filepath.Join(base, "settings.json"),
		fullInterval: time.Minute,
	}
}

// SetSink updates the target sink so a long-lived Scanner remains attached to
// the active sink instance when callers pass a new one into Scan.
func (s *Scanner) SetSink(sink source.Sink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink = sink
}

func (s *Scanner) Scan() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	if n, err := s.scanTranscripts(); err == nil {
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
			n, err := s.scanTrajectory(ap.Port, cascadeID, workspaces)
			if err != nil {
				continue
			}
			total += n
		}
	}
	return total, nil
}

func (s *Scanner) scanTrajectory(port int, cascadeID string, workspaces map[string]string) (int, error) {
	offsetKey := "agy:" + cascadeID + ":gmCount"
	lastCount, _ := s.sink.Offset(offsetKey)

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

		u := model.TokenUsage{
			Source:       antigravity.Source,
			Model:        modelName,
			Project:      project,
			InputTokens:  totalInput,
			CachedTokens: cached,
			OutputTokens: output,
			Timestamp:    ts,
			UUID:         uuid,
		}
		if err := s.sink.Emit(u, "agy://"+cascadeID); err != nil {
			continue
		}
		count++
	}

	if count > 0 || int64(len(gm)) > lastCount {
		s.sink.SetOffset(offsetKey, int64(len(gm)))
	}
	return count, nil
}

func (s *Scanner) loadWorkspaces() map[string]string {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()

	sig, ok := readSignature(s.HistPath)
	if !ok {
		s.workspaces = nil
		s.histValid = false
		return nil
	}
	if s.histValid && s.histSig == sig {
		return s.workspaces
	}
	s.workspaces = readWorkspaces(s.HistPath)
	s.histSig = sig
	s.histValid = true
	return s.workspaces
}

func readWorkspaces(path string) map[string]string {
	f, err := os.Open(path)
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
func (s *Scanner) ScanWithPort(port int) (int, error) {
	workspaces := s.loadWorkspaces()
	summaries, err := getAllTrajectories(port)
	if err != nil {
		return 0, err
	}

	total := 0
	for cascadeID := range summaries {
		n, err := s.scanTrajectory(port, cascadeID, workspaces)
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
		return defaultModelName
	}

	s.metaMu.Lock()
	defer s.metaMu.Unlock()

	sig, ok := readSignature(s.SettingsPath)
	if !ok {
		s.settingsValid = false
		return defaultModelName
	}
	if s.settingsValid && s.settingsSig == sig {
		return s.defaultModel
	}
	s.defaultModel = readDefaultModel(s.SettingsPath)
	s.settingsSig = sig
	s.settingsValid = true
	return s.defaultModel
}

func readDefaultModel(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultModelName
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
	return defaultModelName
}

func readSignature(path string) (fileSignature, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileSignature{}, false
	}
	return fileSignature{modTime: info.ModTime(), size: info.Size()}, true
}

// ScanTranscripts scans the brain directory. It walks the directory at most
// once per fullInterval and otherwise revisits only the transcripts already
// recorded in scan_offsets.
func (s *Scanner) ScanTranscripts() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanTranscripts()
}

func (s *Scanner) scanTranscripts() (int, error) {
	if s.BrainDir == "" {
		return 0, nil
	}
	if _, err := os.Stat(s.BrainDir); os.IsNotExist(err) {
		return 0, nil
	}

	workspaces := s.loadWorkspaces()
	defaultModel := s.loadDefaultModel()

	offsets, hasOffsets := s.allOffsets()
	full := s.shouldFullScan()
	if !hasOffsets {
		full = true
	}
	if full {
		return s.walkTranscripts(workspaces, defaultModel, offsets)
	}
	return s.scanKnownTranscripts(workspaces, defaultModel, offsets)
}

func (s *Scanner) walkTranscripts(workspaces map[string]string, defaultModel string, offsets map[string]int64) (int, error) {
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
		path := transcriptPath(s.BrainDir, cascadeID)
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			continue
		}
		n, err := s.scanTranscriptPath(path, cascadeID, info.Size(), workspaces, defaultModel, offsets)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
}

func (s *Scanner) scanKnownTranscripts(workspaces map[string]string, defaultModel string, offsets map[string]int64) (int, error) {
	total := 0
	for key := range offsets {
		if !strings.HasPrefix(key, transcriptOffsetPrefix) {
			continue
		}
		cascadeID := strings.TrimPrefix(key, transcriptOffsetPrefix)
		path := transcriptPath(s.BrainDir, cascadeID)
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			continue
		}
		n, err := s.scanTranscriptPath(path, cascadeID, info.Size(), workspaces, defaultModel, offsets)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
}

func transcriptPath(brainDir, cascadeID string) string {
	return filepath.Join(brainDir, cascadeID, ".system_generated", "logs", "transcript.jsonl")
}

func (s *Scanner) scanTranscriptPath(path, cascadeID string, size int64, workspaces map[string]string, defaultModel string, offsets map[string]int64) (int, error) {
	offsetKey := transcriptOffsetPrefix + cascadeID
	lastOffset, err := s.offsetFor(offsets, offsetKey)
	if err != nil {
		return 0, err
	}
	if size <= lastOffset {
		return 0, nil
	}

	n, newOffset, err := s.scanTranscriptFile(path, cascadeID, lastOffset, defaultModel, workspaces)
	if err != nil {
		return 0, err
	}
	if newOffset > lastOffset {
		if err := s.sink.SetOffset(offsetKey, newOffset); err != nil {
			return n, err
		}
		if offsets != nil {
			offsets[offsetKey] = newOffset
		}
	}
	return n, nil
}

func (s *Scanner) shouldFullScan() bool {
	interval := s.fullInterval
	if interval <= 0 {
		interval = time.Minute
	}
	if s.lastFull.IsZero() || time.Since(s.lastFull) >= interval {
		s.lastFull = time.Now()
		return true
	}
	return false
}

func (s *Scanner) allOffsets() (map[string]int64, bool) {
	index, ok := s.sink.(source.OffsetIndex)
	if !ok {
		return nil, false
	}
	offsets, err := index.AllOffsets()
	if err != nil {
		return nil, false
	}
	return offsets, true
}

func (s *Scanner) offsetFor(offsets map[string]int64, key string) (int64, error) {
	if offsets != nil {
		return offsets[key], nil
	}
	return s.sink.Offset(key)
}

func (s *Scanner) scanTranscriptFile(path, cascadeID string, fromOffset int64, defaultModel string, workspaces map[string]string) (int, int64, error) {
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

					u := model.TokenUsage{
						Source:       antigravity.Source,
						Model:        currentModel,
						Project:      project,
						InputTokens:  totalInput,
						CachedTokens: cached,
						OutputTokens: output,
						Timestamp:    ts,
						UUID:         uuid,
					}
					if err := s.sink.Emit(u, path); err == nil {
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

