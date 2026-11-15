package antigravity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"token-ray/internal/model"
	"token-ray/internal/watcher"
)

const Source = "Antigravity"

type jsonObject map[string]json.RawMessage

var (
	modelAliases = []string{
		"activeModel",
		"active_model",
		"model",
		"modelName",
		"model_name",
	}
	cwdAliases = []string{
		"cwd",
		"currentWorkingDirectory",
		"current_working_directory",
		"workspacePath",
		"workspace_path",
		"currentDir",
		"current_dir",
	}
	projectDirAliases = []string{
		"projectDir",
		"project_dir",
	}
	workspacePathsAliases = []string{
		"workspacePaths",
		"workspace_paths",
	}
	conversationAliases = []string{
		"conversationId",
		"conversation_id",
		"sessionId",
		"session_id",
		"conversationUuid",
		"conversation_uuid",
	}
	transcriptAliases = []string{
		"transcriptPath",
		"transcript_path",
	}
	inputAliases = []string{
		"inputTokens",
		"input_tokens",
		"promptTokens",
		"prompt_tokens",
		"promptTokenCount",
		"prompt_token_count",
		"numPromptTokens",
		"num_prompt_tokens",
	}
	cachedAliases = []string{
		"cachedTokens",
		"cached_tokens",
		"cachedContentTokens",
		"cached_content_tokens",
		"cachedContentTokenCount",
		"cached_content_token_count",
		"cacheReadInputTokens",
		"cache_read_input_tokens",
	}
	cacheCreationAliases = []string{
		"cacheCreationTokens",
		"cache_creation_tokens",
		"cacheCreationInputTokens",
		"cache_creation_input_tokens",
		"cacheWriteTokens",
		"cache_write_tokens",
	}
	outputAliases = []string{
		"outputTokens",
		"output_tokens",
		"completionTokens",
		"completion_tokens",
		"candidatesTokenCount",
		"candidates_token_count",
		"responseTokens",
		"response_tokens",
	}
	thoughtsAliases = []string{
		"thoughtsTokens",
		"thoughts_tokens",
		"thoughtsTokenCount",
		"thoughts_token_count",
		"reasoningTokens",
		"reasoning_tokens",
		"reasoningOutputTokens",
		"reasoning_output_tokens",
	}
	totalAliases = []string{
		"totalTokens",
		"total_tokens",
		"totalTokenCount",
		"total_token_count",
	}
)

func ParseStatusline(raw []byte, now time.Time) (model.TokenUsage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return model.TokenUsage{}, false, fmt.Errorf("empty Antigravity statusline payload")
	}

	var root jsonObject
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return model.TokenUsage{}, false, fmt.Errorf("decode Antigravity statusline JSON: %w", err)
	}

	objects := collectObjects(json.RawMessage(trimmed), 6)
	input, hasInput := firstInt(objects, inputAliases)
	cached, hasCached := firstInt(objects, cachedAliases)
	cacheCreation, hasCacheCreation := firstInt(objects, cacheCreationAliases)
	output, hasOutput := firstInt(objects, outputAliases)
	thoughts, hasThoughts := firstInt(objects, thoughtsAliases)
	total, hasTotal := firstInt(objects, totalAliases)

	if !hasInput && !hasCached && !hasCacheCreation && !hasOutput && !hasThoughts && !hasTotal {
		return model.TokenUsage{}, false, nil
	}

	if !hasInput && hasTotal && (hasOutput || hasThoughts) {
		input = total - output - thoughts
		if input < 0 {
			input = 0
		}
	}
	if !hasInput && hasTotal && !hasOutput && !hasThoughts {
		input = total
	}
	if hasInput && usesSplitCacheTokenFields(objects) {
		input += cached + cacheCreation
	}

	outputTotal := output + thoughts
	if hasTotal && input > 0 && total > input+outputTotal {
		outputTotal = total - input
	}
	if outputTotal < 0 {
		outputTotal = 0
	}
	if input == 0 && cached == 0 && cacheCreation == 0 && outputTotal == 0 && thoughts == 0 {
		return model.TokenUsage{}, false, nil
	}

	modelName := normalizeModelName(firstString(objects, modelAliases))
	if modelName == "" {
		modelName = "unknown"
	}
	project := watcher.ExtractProjectNameFromCWD(firstWorkspacePath(objects))
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return model.TokenUsage{
		Source:              Source,
		Model:               modelName,
		Project:             project,
		InputTokens:         input,
		CachedTokens:        cached,
		CacheCreationTokens: cacheCreation,
		OutputTokens:        outputTotal,
		Thoughts:            thoughts,
		Timestamp:           now.UTC(),
		UUID:                statuslineUUID(objects, trimmed),
	}, true, nil
}

func TotalTokens(usage model.TokenUsage) int {
	return usage.InputTokens + usage.OutputTokens
}

func collectObjects(raw json.RawMessage, depth int) []jsonObject {
	if depth <= 0 {
		return nil
	}
	var obj jsonObject
	if err := json.Unmarshal(raw, &obj); err == nil {
		objects := []jsonObject{obj}
		for _, child := range obj {
			objects = append(objects, collectObjects(child, depth-1)...)
		}
		return objects
	}

	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		var objects []jsonObject
		for _, child := range list {
			objects = append(objects, collectObjects(child, depth-1)...)
		}
		return objects
	}
	return nil
}

func firstWorkspacePath(objects []jsonObject) string {
	if projectDir := firstString(objects, projectDirAliases); projectDir != "" {
		return projectDir
	}
	if workspacePath := firstStringFromArray(objects, workspacePathsAliases); workspacePath != "" {
		return workspacePath
	}
	return firstString(objects, cwdAliases)
}

func firstString(objects []jsonObject, aliases []string) string {
	for _, obj := range objects {
		for _, alias := range aliases {
			raw, ok := lookupRaw(obj, alias)
			if !ok {
				continue
			}
			value, ok := rawString(raw)
			if ok && value != "" {
				return value
			}
		}
	}
	return ""
}

func firstStringFromArray(objects []jsonObject, aliases []string) string {
	for _, obj := range objects {
		for _, alias := range aliases {
			raw, ok := lookupRaw(obj, alias)
			if !ok {
				continue
			}
			var values []string
			if err := json.Unmarshal(raw, &values); err == nil {
				for _, value := range values {
					value = strings.TrimSpace(value)
					if value != "" {
						return value
					}
				}
			}
		}
	}
	return ""
}

func firstInt(objects []jsonObject, aliases []string) (int, bool) {
	for _, obj := range objects {
		for _, alias := range aliases {
			raw, ok := lookupRaw(obj, alias)
			if !ok {
				continue
			}
			value, ok := rawInt(raw)
			if ok {
				return value, true
			}
		}
	}
	return 0, false
}

func usesSplitCacheTokenFields(objects []jsonObject) bool {
	for _, obj := range objects {
		for _, alias := range []string{
			"cacheReadInputTokens",
			"cache_read_input_tokens",
			"cacheCreationInputTokens",
			"cache_creation_input_tokens",
		} {
			if _, ok := lookupRaw(obj, alias); ok {
				return true
			}
		}
	}
	return false
}

func lookupRaw(obj jsonObject, alias string) (json.RawMessage, bool) {
	normalizedAlias := normalizeKey(alias)
	for key, raw := range obj {
		if normalizeKey(key) == normalizedAlias {
			return raw, true
		}
	}
	return nil, false
}

func rawString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return rawObjectString(raw)
	}
	return strings.TrimSpace(value), true
}

func rawObjectString(raw json.RawMessage) (string, bool) {
	var obj jsonObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", false
	}
	for _, alias := range []string{"id", "displayName", "display_name", "name"} {
		nested, ok := lookupRaw(obj, alias)
		if !ok {
			continue
		}
		value, ok := rawString(nested)
		if ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func normalizeModelName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "gemini ") && !strings.HasPrefix(lower, "gemini-") {
		return value
	}

	slug := slugModelName(stripParenthetical(value))
	for _, suffix := range []string{"-high", "-medium", "-low"} {
		slug = strings.TrimSuffix(slug, suffix)
	}
	return slug
}

func stripParenthetical(value string) string {
	var builder strings.Builder
	depth := 0
	for _, r := range value {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func slugModelName(value string) string {
	var builder strings.Builder
	lastSeparator := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.':
			builder.WriteRune(r)
			lastSeparator = false
		default:
			if builder.Len() > 0 && !lastSeparator {
				builder.WriteByte('-')
				lastSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func rawInt(raw json.RawMessage) (int, bool) {
	if value, ok := rawString(raw); ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var number json.Number
	if err := decoder.Decode(&number); err != nil {
		return 0, false
	}
	if parsed, err := number.Int64(); err == nil {
		return int(parsed), true
	}
	parsed, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return 0, false
	}
	return int(parsed), true
}

func statuslineUUID(objects []jsonObject, raw []byte) string {
	if conversationID := firstString(objects, conversationAliases); conversationID != "" {
		return "antigravity-statusline:" + conversationID
	}
	if transcriptPath := firstString(objects, transcriptAliases); transcriptPath != "" {
		return "antigravity-statusline:transcript:" + shortHash(transcriptPath)
	}
	return "antigravity-statusline:payload:" + shortHash(string(raw))
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func normalizeKey(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r == '_' || r == '-' || unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(unicode.ToLower(r))
	}
	return builder.String()
}
