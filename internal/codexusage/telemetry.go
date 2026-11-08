package codexusage

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

var metricPattern = regexp.MustCompile(`([A-Za-z_.]+)=("[^"]*"|\S+)`)

type codexEvent struct {
	logID        int64
	ts           time.Time
	conversation string
	model        string
	input        int
	cached       int
	output       int
	reasoning    int
}

func (s *Scanner) scanLogs(threads map[string]threadInfo) (int, error) {
	if len(threads) == 0 || s.LogsPath == "" {
		return 0, nil
	}
	if _, err := os.Stat(s.LogsPath); err != nil {
		return 0, nil
	}

	offset, err := s.sink.Offset(OffsetKey)
	if err != nil {
		return 0, err
	}

	logConn, err := openReadonlySQLite(s.LogsPath)
	if err != nil {
		return 0, err
	}
	defer logConn.Close()
	hasLogs, err := hasTable(logConn, "logs")
	if err != nil {
		if isOptionalCodexDBError(err) {
			return 0, nil
		}
		return 0, err
	}
	if !hasLogs {
		return 0, nil
	}

	rows, err := logConn.Query(`
			SELECT id, ts, ts_nanos, feedback_log_body
			FROM logs
			WHERE id > ?
				AND target = 'codex_otel.log_only'
				AND (
					feedback_log_body LIKE 'event.name="codex.sse_event" event.kind=response.completed% input_token_count=%'
					OR feedback_log_body LIKE 'event.name=codex.sse_event event.kind=response.completed% input_token_count=%'
					OR feedback_log_body LIKE '%: event.name="codex.sse_event" event.kind=response.completed% input_token_count=%'
					OR feedback_log_body LIKE '%: event.name=codex.sse_event event.kind=response.completed% input_token_count=%'
				)
			ORDER BY id ASC
		`, offset)
	if err != nil {
		if isOptionalCodexDBError(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	count := 0
	maxOffset := offset
	for rows.Next() {
		var logID, tsSec, tsNanos int64
		var body string
		if err := rows.Scan(&logID, &tsSec, &tsNanos, &body); err != nil {
			if isOptionalCodexDBError(err) {
				return count, nil
			}
			return count, err
		}
		event, ok := parseCodexEvent(logID, tsSec, tsNanos, body)
		if !ok {
			if logID > maxOffset {
				maxOffset = logID
			}
			continue
		}
		info, ok := threads[event.conversation]
		if !ok {
			break
		}
		modelName := event.model
		if modelName == "" {
			modelName = info.model
		}
		if modelName == "" {
			modelName = "codex"
		}

		u := model.TokenUsage{
			Source:       SourceName,
			Model:        modelName,
			Project:      watcher.ExtractProjectNameFromCWD(info.cwd),
			InputTokens:  event.input,
			CachedTokens: event.cached,
			OutputTokens: event.output,
			Thoughts:     event.reasoning,
			Timestamp:    event.ts,
			UUID:         fmt.Sprintf("codex:%d", event.logID),
		}
		if err := s.sink.Emit(u, s.LogsPath); err != nil {
			return count, err
		}
		if logID > maxOffset {
			maxOffset = logID
		}
		count++
	}
	if err := rows.Err(); err != nil {
		if isOptionalCodexDBError(err) {
			return count, nil
		}
		return count, err
	}
	if maxOffset > offset {
		if err := s.sink.SetOffset(OffsetKey, maxOffset); err != nil {
			return count, err
		}
	}
	return count, nil
}

func parseCodexEvent(logID int64, tsSec int64, tsNanos int64, body string) (codexEvent, bool) {
	eventBody, ok := codexTelemetryEventBody(body)
	if !ok {
		return codexEvent{}, false
	}
	fields := parseMetrics(eventBody)
	if fields["event.name"] != "codex.sse_event" || fields["event.kind"] != "response.completed" {
		return codexEvent{}, false
	}
	input := parseIntField(fields, "input_token_count")
	output := parseIntField(fields, "output_token_count")
	cached := parseIntField(fields, "cached_token_count")
	reasoning := parseIntField(fields, "reasoning_token_count")
	conversation := fields["conversation.id"]
	if conversation == "" || input == 0 && output == 0 && cached == 0 {
		return codexEvent{}, false
	}

	ts := time.Unix(tsSec, tsNanos).UTC()
	if eventTS := fields["event.timestamp"]; eventTS != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, eventTS); err == nil {
			ts = parsed.UTC()
		}
	}

	return codexEvent{
		logID:        logID,
		ts:           ts,
		conversation: conversation,
		model:        fields["model"],
		input:        input,
		cached:       cached,
		output:       output,
		reasoning:    reasoning,
	}, true
}

func codexTelemetryEventBody(body string) (string, bool) {
	if strings.HasPrefix(body, "event.name=") {
		return body, true
	}
	const prefixedEventMarker = ": event.name="
	idx := strings.Index(body, prefixedEventMarker)
	if idx == -1 {
		return "", false
	}
	return body[idx+2:], true
}

func parseMetrics(body string) map[string]string {
	fields := make(map[string]string)
	for _, match := range metricPattern.FindAllStringSubmatch(body, -1) {
		if _, exists := fields[match[1]]; exists {
			continue
		}
		value := match[2]
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		fields[match[1]] = value
	}
	return fields
}

func parseIntField(fields map[string]string, key string) int {
	value, err := strconv.Atoi(fields[key])
	if err != nil {
		return 0
	}
	return value
}
