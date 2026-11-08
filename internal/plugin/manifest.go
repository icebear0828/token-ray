package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const ManifestName = "manifest.json"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// reservedSources keeps plugins from silently merging their data into a
// built-in source's statistics.
var reservedSources = map[string]bool{
	"Claude Code":      true,
	"Gemini CLI":       true,
	"Codex":            true,
	"Antigravity":      true,
	"DeepSeek Harness": true,
	"Pi Agent":         true,
}

var reservedIDs = map[string]bool{
	"claude-code": true,
	"gemini-cli":  true,
	"codex":       true,
	"antigravity": true,
	"extra-jsonl": true,
	"dsh":         true,
	"pi-agent":    true,
}

// Manifest is the declarative description of a source plugin. It contains no
// executable code: the host maps log fields onto TokenUsage.
type Manifest struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Source        string   `json:"source"`
	Label         string   `json:"label,omitempty"`
	Version       string   `json:"version"`
	MinAppVersion string   `json:"min_app_version,omitempty"`
	Description   string   `json:"description,omitempty"`
	Author        string   `json:"author,omitempty"`
	Homepage      string   `json:"homepage,omitempty"`
	WatchDirs     []string `json:"watch_dirs"`
	FileGlobs     []string `json:"file_globs,omitempty"`
	Match         *Match   `json:"match,omitempty"`
	Fields        FieldMap `json:"fields"`
	InputMode     string   `json:"input_mode,omitempty"`
	TimestampFmt  string   `json:"timestamp_format,omitempty"`
	Dedup         string   `json:"dedup,omitempty"`
}

// Match selects which JSON lines are usage records. A nil Match accepts every
// line that maps to a non-empty model and at least one token count.
type Match struct {
	Field  string `json:"field"`
	Equals any    `json:"equals,omitempty"`
	Exists bool   `json:"exists,omitempty"`
}

// FieldMap uses dot-separated paths into the JSON object.
type FieldMap struct {
	Model               string `json:"model"`
	InputTokens         string `json:"input_tokens"`
	CachedTokens        string `json:"cached_tokens,omitempty"`
	CacheCreationTokens string `json:"cache_creation_tokens,omitempty"`
	OutputTokens        string `json:"output_tokens,omitempty"`
	Thoughts            string `json:"thoughts,omitempty"`
	Timestamp           string `json:"timestamp,omitempty"`
	UUID                string `json:"uuid,omitempty"`
	Project             string `json:"project,omitempty"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d (want 1)", m.SchemaVersion)
	}
	if !idPattern.MatchString(m.ID) {
		return fmt.Errorf("invalid id %q: use lowercase letters, digits, '.', '_' or '-'", m.ID)
	}
	if reservedIDs[m.ID] {
		return fmt.Errorf("id %q is reserved by a built-in source", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if reservedSources[m.Source] {
		return fmt.Errorf("source %q is reserved by a built-in source", m.Source)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if len(m.WatchDirs) == 0 {
		return fmt.Errorf("watch_dirs must contain at least one directory")
	}
	if len(m.WatchDirs) > 20 {
		return fmt.Errorf("watch_dirs supports at most 20 directories")
	}
	if strings.TrimSpace(m.Fields.Model) == "" {
		return fmt.Errorf("fields.model is required")
	}
	if strings.TrimSpace(m.Fields.InputTokens) == "" {
		return fmt.Errorf("fields.input_tokens is required")
	}
	switch m.InputMode {
	case "", "total", "exclusive":
	default:
		return fmt.Errorf("input_mode must be \"total\" or \"exclusive\"")
	}
	switch m.Dedup {
	case "", "line", "none":
	default:
		return fmt.Errorf("dedup must be \"line\" or \"none\"")
	}
	if m.Match != nil && strings.TrimSpace(m.Match.Field) == "" {
		return fmt.Errorf("match.field is required when match is set")
	}
	if len(m.FileGlobs) > 20 {
		return fmt.Errorf("file_globs supports at most 20 patterns")
	}
	return nil
}

// ExpandWatchDirs expands ~ and environment variables and restricts every
// directory to the user's home tree. A plugin must never be able to ask the
// host to scan arbitrary filesystem roots.
func (m Manifest) ExpandWatchDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, fmt.Errorf("cannot resolve user home directory")
	}
	home = filepath.Clean(home)
	out := make([]string, 0, len(m.WatchDirs))
	for _, raw := range m.WatchDirs {
		dir := strings.TrimSpace(raw)
		if dir == "" {
			continue
		}
		dir = os.ExpandEnv(dir)
		if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
			dir = strings.TrimPrefix(dir, "~")
			dir = strings.TrimPrefix(dir, "/")
			dir = strings.TrimPrefix(dir, `\`)
			dir = filepath.Join(home, dir)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("watch dir %q: %w", raw, err)
		}
		rel, err := filepath.Rel(home, abs)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("watch dir %q must be inside the user home directory", raw)
		}
		out = append(out, abs)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("watch_dirs must contain at least one directory")
	}
	return out, nil
}

func (m Manifest) LabelText() string {
	if strings.TrimSpace(m.Label) != "" {
		return m.Label
	}
	return strings.ToUpper(m.Source)
}

// Matcher returns a file-name predicate. Defaults to *.jsonl.
func (m Manifest) Matcher() func(string) bool {
	globs := m.FileGlobs
	if len(globs) == 0 {
		globs = []string{"*.jsonl"}
	}
	return func(path string) bool {
		base := filepath.Base(path)
		for _, glob := range globs {
			if ok, err := filepath.Match(glob, base); err == nil && ok {
				return true
			}
		}
		return false
	}
}

// versionAtLeast reports whether current >= minimum. "dev" builds skip the
// check so local development is not blocked by plugin metadata.
func versionAtLeast(current, minimum string) bool {
	if minimum == "" || current == "" || current == "dev" {
		return true
	}
	cur := parseVersion(current)
	min := parseVersion(minimum)
	for i := 0; i < len(cur) || i < len(min); i++ {
		var a, b int
		if i < len(cur) {
			a = cur[i]
		}
		if i < len(min) {
			b = min[i]
		}
		if a != b {
			return a > b
		}
	}
	return true
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}

func decodeManifest(data []byte) (Manifest, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("decode manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return m, err
	}
	return m, nil
}
