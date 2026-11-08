package dsh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

type dshUsage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CacheReadTokens  int `json:"cacheReadTokens"`
	CacheWriteTokens int `json:"cacheWriteTokens"`
	ReasoningTokens  int `json:"reasoningTokens"`
}

type dshStreamEntry struct {
	Chunk struct {
		Type  string    `json:"type"`
		Usage *dshUsage `json:"usage"`
	} `json:"chunk"`
}

type dshRecord struct {
	Type       string  `json:"type"`
	Seq        *int    `json:"seq"`
	Time       int64   `json:"time"`
	ID         string  `json:"id"`
	Cwd        string  `json:"cwd"`
	Model      string  `json:"model"`
	Provider   string  `json:"provider"`
	SeedLength *int    `json:"seedLength"`
	IsSeeded   *bool   `json:"isSeeded"`
	Data       struct {
		Usage     *dshUsage       `json:"usage"`
		Inherited bool            `json:"inherited"`
		Stream    []dshStreamEntry `json:"stream"`
		Message   struct {
			ID     string `json:"id"`
			Source struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			} `json:"source"`
		} `json:"message"`
	} `json:"data"`
}

type dshUsageCandidate struct {
	recordType  string
	seq         *int
	timeMs      int64
	afterHeader bool
	identity    string
	model       string
	usage       *dshUsage
}

func parseTranscriptFile(path string, compressed bool, fallbackSessionID string) ([]model.TokenUsage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reader io.Reader = file
	if compressed {
		decoder, err := zstd.NewReader(file, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		defer decoder.Close()
		reader = decoder
	}
	return parseTranscript(reader, fallbackSessionID)
}

func parseTranscript(reader io.Reader, fallbackSessionID string) ([]model.TokenUsage, error) {
	decoder := json.NewDecoder(reader)

	var header *dshRecord
	seenHeader := false
	lastInheritedSeq := -1
	modelName := ""
	recordIndex := 0
	candidates := make([]dshUsageCandidate, 0, 64)

	for {
		var record dshRecord
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, err
		}
		recordIndex++

		switch record.Type {
		case "session":
			if header == nil {
				copied := record
				header = &copied
			}
			seenHeader = true
			if record.Model != "" {
				modelName = record.Model
			}
			continue
		case "session/end-seed":
			if record.Data.Inherited && record.Seq != nil {
				lastInheritedSeq = *record.Seq
			}
			continue
		}

		if record.Data.Message.Source.Model != "" {
			modelName = record.Data.Message.Source.Model
		}
		usage := dshRecordUsage(record)
		if usage == nil || record.Time <= 0 {
			continue
		}
		if usage.InputTokens+usage.OutputTokens+usage.CacheReadTokens+usage.CacheWriteTokens == 0 {
			continue
		}

		identity := ""
		if record.Data.Message.ID != "" {
			identity = "msg:" + record.Data.Message.ID
		} else if record.Seq != nil {
			identity = fmt.Sprintf("seq:%d", *record.Seq)
		} else {
			identity = fmt.Sprintf("record:%d", recordIndex)
		}
		if record.Type == "compaction/summary" {
			identity = "summary:" + identity
		}
		if record.Type == "assistant/attempt" {
			identity = "attempt:" + identity
		}

		candidates = append(candidates, dshUsageCandidate{
			recordType:  record.Type,
			seq:         record.Seq,
			timeMs:      record.Time,
			afterHeader: seenHeader,
			identity:    identity,
			model:       modelName,
			usage:       usage,
		})
	}

	sessionID := fallbackSessionID
	if header != nil && header.ID != "" {
		sessionID = header.ID
	}
	project := "Default"
	if header != nil && header.Cwd != "" {
		project = watcher.ExtractProjectNameFromCWD(header.Cwd)
	}

	inheritedCut := -1
	hasLegacyCut := header != nil && header.SeedLength != nil
	if hasLegacyCut {
		inheritedCut = *header.SeedLength
	}
	needsTaggedCut := header != nil && !hasLegacyCut && header.IsSeeded != nil && *header.IsSeeded
	if needsTaggedCut && lastInheritedSeq >= 0 {
		inheritedCut = lastInheritedSeq
	}

	seen := make(map[string]int, len(candidates))
	out := make([]model.TokenUsage, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.afterHeader {
			if needsTaggedCut && inheritedCut < 0 {
				continue
			}
			if inheritedCut >= 0 && candidate.seq != nil && *candidate.seq < inheritedCut {
				continue
			}
		}

		modelValue := strings.TrimSpace(candidate.model)
		if modelValue == "" {
			modelValue = "unknown"
		}
		uuid := dshUUID(sessionID, candidate.recordType, candidate.identity, candidate.timeMs)
		usage := model.TokenUsage{
			Source:              sourceName,
			Model:               modelValue,
			Project:             project,
			InputTokens:         candidate.usage.InputTokens + candidate.usage.CacheReadTokens + candidate.usage.CacheWriteTokens,
			CachedTokens:        candidate.usage.CacheReadTokens,
			CacheCreationTokens: candidate.usage.CacheWriteTokens,
			OutputTokens:        candidate.usage.OutputTokens,
			Thoughts:            candidate.usage.ReasoningTokens,
			Timestamp:           time.UnixMilli(candidate.timeMs).UTC(),
			UUID:                uuid,
		}
		// A replayed line can carry corrected usage. Keep the last occurrence,
		// matching the scanner's UUID dedup and the DB upsert semantics.
		if index, ok := seen[uuid]; ok {
			out[index] = usage
			continue
		}
		seen[uuid] = len(out)
		out = append(out, usage)
	}
	return out, nil
}

func dshRecordUsage(record dshRecord) *dshUsage {
	switch record.Type {
	case "assistant/message":
		if record.Data.Usage != nil {
			return record.Data.Usage
		}
		return lastStreamUsage(record.Data.Stream)
	case "assistant/attempt":
		return lastStreamUsage(record.Data.Stream)
	case "compaction/summary":
		if record.Data.Usage != nil {
			return record.Data.Usage
		}
		return lastStreamUsage(record.Data.Stream)
	default:
		return nil
	}
}

func lastStreamUsage(stream []dshStreamEntry) *dshUsage {
	for i := len(stream) - 1; i >= 0; i-- {
		if stream[i].Chunk.Usage != nil {
			return stream[i].Chunk.Usage
		}
	}
	return nil
}

// dshUUID identifies the event entity, not its payload. Token counts, provider
// and model are deliberately excluded so a replayed or corrected record upserts
// the same row instead of inserting a second one.
func dshUUID(sessionID, recordType, identity string, timeMs int64) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%d", sessionID, recordType, identity, timeMs)
	return "dsh:" + hex.EncodeToString(h.Sum(nil))
}
