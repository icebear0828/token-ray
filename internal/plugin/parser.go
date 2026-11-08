package plugin

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

// parser maps one JSON line onto TokenUsage using a declarative manifest.
type parser struct {
	manifest Manifest
}

func (p *parser) Parse(line string) (model.TokenUsage, string, bool) {
	line = strings.TrimPrefix(line, string(rune(0xFEFF)))
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return model.TokenUsage{}, "", false
	}
	if !p.matches(obj) {
		return model.TokenUsage{}, "", false
	}

	modelName := asString(valueAt(obj, p.manifest.Fields.Model))
	input := asInt(valueAt(obj, p.manifest.Fields.InputTokens))
	cached := asInt(valueAt(obj, p.manifest.Fields.CachedTokens))
	cacheCreation := asInt(valueAt(obj, p.manifest.Fields.CacheCreationTokens))
	output := asInt(valueAt(obj, p.manifest.Fields.OutputTokens))
	thoughts := asInt(valueAt(obj, p.manifest.Fields.Thoughts))

	if input == 0 && cached == 0 && cacheCreation == 0 && output == 0 {
		return model.TokenUsage{}, "", false
	}
	if p.manifest.InputMode == "exclusive" {
		input += cached + cacheCreation
	}

	u := model.TokenUsage{
		Source:              p.manifest.Source,
		Model:               modelName,
		InputTokens:         input,
		CachedTokens:        cached,
		CacheCreationTokens: cacheCreation,
		OutputTokens:        output,
		Thoughts:            thoughts,
	}

	projectHint := ""
	if p.manifest.Fields.Project != "" {
		if raw := asString(valueAt(obj, p.manifest.Fields.Project)); raw != "" {
			projectHint = watcher.ExtractProjectNameFromCWD(raw)
			u.Project = projectHint
		}
	}

	if p.manifest.Fields.Timestamp != "" {
		u.Timestamp = parseTime(valueAt(obj, p.manifest.Fields.Timestamp), p.manifest.TimestampFmt)
	}
	if p.manifest.Fields.UUID != "" {
		u.UUID = asString(valueAt(obj, p.manifest.Fields.UUID))
	}
	if u.Model == "" {
		u.Model = "unknown"
	}
	return u, projectHint, true
}

func (p *parser) ProjectFromPath(string) string { return "Default" }

func (p *parser) FallbackUUID(deviceID, path string, lineStartOffset int64, _ model.TokenUsage) string {
	if p.manifest.Dedup == "none" {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d", p.manifest.ID, deviceID, filepath.ToSlash(path), lineStartOffset)
	return fmt.Sprintf("plugin:%s:%x", p.manifest.ID, h.Sum(nil)[:12])
}

func (p *parser) matches(obj map[string]any) bool {
	if p.manifest.Match == nil {
		return true
	}
	value, ok := lookup(obj, p.manifest.Match.Field)
	if p.manifest.Match.Exists {
		return ok
	}
	if !ok {
		return false
	}
	if p.manifest.Match.Equals == nil {
		return true
	}
	return fmt.Sprint(value) == fmt.Sprint(p.manifest.Match.Equals)
}

func valueAt(obj map[string]any, path string) any {
	v, _ := lookup(obj, path)
	return v
}

func lookup(obj map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	var cur any = obj
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

func asInt(v any) int {
	switch value := v.(type) {
	case json.Number:
		if i, err := value.Int64(); err == nil {
			return int(i)
		}
		if f, err := value.Float64(); err == nil {
			return int(f)
		}
	case float64:
		return int(value)
	case int:
		return value
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return i
		}
	}
	return 0
}

func parseTime(v any, layout string) time.Time {
	if v == nil {
		return time.Time{}
	}
	if layout != "" {
		if s := asString(v); s != "" {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC()
			}
		}
		return time.Time{}
	}
	switch value := v.(type) {
	case json.Number:
		return unixTime(value.String())
	case float64:
		return unixTime(strconv.FormatFloat(value, 'f', -1, 64))
	case string:
		for _, candidate := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000Z",
			"2006-01-02 15:04:05",
		} {
			if t, err := time.Parse(candidate, value); err == nil {
				return t.UTC()
			}
		}
		return unixTime(value)
	}
	return time.Time{}
}

func unixTime(raw string) time.Time {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return time.Time{}
	}
	if f > 1e12 {
		return time.UnixMilli(int64(f)).UTC()
	}
	return time.Unix(int64(f), 0).UTC()
}
