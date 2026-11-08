package dsh

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/testutil"
)

const (
	headerLine = `{"type":"session","id":"s1","cwd":"/work/demo"}`
	usageOne   = `{"type":"assistant/message","seq":2,"time":1700000000000,"data":{"usage":{"inputTokens":100,"outputTokens":20,"cacheReadTokens":50,"cacheWriteTokens":10,"reasoningTokens":5},"message":{"id":"m1","source":{"provider":"deepseek","model":"deepseek-v4-pro"}}}}`
	usageTwo   = `{"type":"assistant/message","seq":3,"time":1700000001000,"data":{"usage":{"inputTokens":200,"outputTokens":40},"message":{"id":"m2","source":{"provider":"deepseek","model":"deepseek-v4-pro"}}}}`
)

func transcriptText(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func compressZstd(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeSessionBytes(t *testing.T, root, project, session, name string, content []byte) string {
	t.Helper()
	dir := filepath.Join(root, "sessions", project, session)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestParseTranscriptMapsUsage(t *testing.T) {
	text := transcriptText(
		headerLine,
		usageOne,
		`{"type":"assistant/attempt","seq":3,"time":1700000002000,"data":{"stream":[{"chunk":{"type":"usage","usage":{"inputTokens":10,"outputTokens":5}}}]}}`,
		`{"type":"compaction/summary","seq":4,"time":1700000003000,"data":{"usage":{"inputTokens":40,"outputTokens":10},"message":{"source":{"model":"deepseek-v4-flash"}}}}`,
		`{"type":"assistant/message","seq":5,"time":0,"data":{"usage":{"inputTokens":99,"outputTokens":99}}}`,
		`{"type":"assistant/message","seq":6,"time":1700000004000,"data":{"usage":{"inputTokens":0,"outputTokens":0}}}`,
	)

	usages, err := parseTranscript(strings.NewReader(text), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 3 {
		t.Fatalf("expected three usage records, got %+v", usages)
	}

	first := usages[0]
	if first.Model != "deepseek-v4-pro" || first.Project != "demo" {
		t.Fatalf("unexpected first model/project: %+v", first)
	}
	if first.InputTokens != 160 || first.CachedTokens != 50 || first.CacheCreationTokens != 10 || first.OutputTokens != 20 || first.Thoughts != 5 {
		t.Fatalf("unexpected first token mapping: %+v", first)
	}
	if first.UUID == "" {
		t.Fatal("expected a stable uuid")
	}
	if !first.Timestamp.Equal(time.UnixMilli(1700000000000).UTC()) {
		t.Fatalf("unexpected first timestamp: %s", first.Timestamp)
	}

	second := usages[1]
	if second.Model != "deepseek-v4-pro" || second.InputTokens != 10 || second.OutputTokens != 5 {
		t.Fatalf("assistant/attempt should inherit the model: %+v", second)
	}

	third := usages[2]
	if third.Model != "deepseek-v4-flash" || third.InputTokens != 40 || third.OutputTokens != 10 {
		t.Fatalf("unexpected compaction usage: %+v", third)
	}
}

func TestParseTranscriptSkipsInheritedUsage(t *testing.T) {
	t.Run("seedLength", func(t *testing.T) {
		text := transcriptText(
			`{"type":"session","id":"s1","seedLength":2}`,
			`{"type":"assistant/message","seq":1,"time":1000,"data":{"usage":{"inputTokens":100,"outputTokens":10}}}`,
			`{"type":"assistant/message","seq":2,"time":2000,"data":{"usage":{"inputTokens":200,"outputTokens":20}}}`,
		)
		usages, err := parseTranscript(strings.NewReader(text), "fallback")
		if err != nil {
			t.Fatal(err)
		}
		if len(usages) != 1 || usages[0].InputTokens != 200 {
			t.Fatalf("expected only the post-seed usage, got %+v", usages)
		}
	})

	t.Run("tagged end seed", func(t *testing.T) {
		text := transcriptText(
			`{"type":"session","id":"s1","isSeeded":true}`,
			`{"type":"assistant/message","seq":1,"time":1000,"data":{"usage":{"inputTokens":100,"outputTokens":10}}}`,
			`{"type":"session/end-seed","seq":2,"time":1500,"data":{"inherited":true}}`,
			`{"type":"assistant/message","seq":3,"time":2000,"data":{"usage":{"inputTokens":200,"outputTokens":20}}}`,
		)
		usages, err := parseTranscript(strings.NewReader(text), "fallback")
		if err != nil {
			t.Fatal(err)
		}
		if len(usages) != 1 || usages[0].InputTokens != 200 {
			t.Fatalf("expected only the post-seed usage, got %+v", usages)
		}
	})

	t.Run("missing tagged cut", func(t *testing.T) {
		text := transcriptText(
			`{"type":"session","id":"s1","isSeeded":true}`,
			`{"type":"assistant/message","seq":1,"time":1000,"data":{"usage":{"inputTokens":100,"outputTokens":10}}}`,
		)
		usages, err := parseTranscript(strings.NewReader(text), "fallback")
		if err != nil {
			t.Fatal(err)
		}
		if len(usages) != 0 {
			t.Fatalf("expected no usage when the seed cut is unknown, got %+v", usages)
		}
	})
}

func TestParseTranscriptDeduplicatesReplayedRecords(t *testing.T) {
	text := transcriptText(headerLine, usageOne, usageOne)
	usages, err := parseTranscript(strings.NewReader(text), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected replayed lines to dedup, got %+v", usages)
	}
}

func TestPreferredTranscriptSelectsVersionedZstd(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"session.jsonl", "session.v2.jsonl", "session.v3.jsonl.zstd"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, ok := preferredTranscript(dir)
	if !ok || got.name != "session.v3.jsonl.zstd" {
		t.Fatalf("expected the versioned zstd transcript, got %+v", got)
	}
}

func TestPreferredTranscriptPrefersCompressedAtSameVersionAndTime(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "session.v3.jsonl")
	compressed := filepath.Join(dir, "session.v3.jsonl.zstd")
	for _, path := range []string{plain, compressed} {
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stamp := time.Unix(1700000000, 0)
	if err := os.Chtimes(plain, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(compressed, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	got, ok := preferredTranscript(dir)
	if !ok || got.name != "session.v3.jsonl.zstd" {
		t.Fatalf("expected the compressed transcript, got %+v", got)
	}
}

func TestSourceScansZstdAndDedups(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DSH_HOME", root)
	writeSessionBytes(t, root, "proj", "s1", "session.v3.jsonl.zstd", compressZstd(t, transcriptText(headerLine, usageOne)))

	database, calc := testutil.NewTestDBAndCalc(t)
	sink := testutil.NewTestSink(t, database, calc, "local")
	src := New("")

	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err := database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 1 || stats[0].InputTokens != 160 {
		t.Fatalf("unexpected stats after zstd scan: %+v", stats)
	}

	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err = database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 1 {
		t.Fatalf("rescan must not duplicate rows, got %+v", stats)
	}
}

func TestSourceScansPlainIncrementally(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DSH_HOME", root)
	first := transcriptText(headerLine, usageOne)
	file := writeSessionBytes(t, root, "proj", "s1", "session.jsonl", []byte(first))

	database, calc := testutil.NewTestDBAndCalc(t)
	sink := testutil.NewTestSink(t, database, calc, "local")
	src := New("")

	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(first+usageTwo+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 2 {
		t.Fatalf("expected two unique rows after append, got %+v", stats)
	}
}

func TestDshUUIDIgnoresUsageAndKeepsLast(t *testing.T) {
	text := transcriptText(
		headerLine,
		`{"type":"assistant/message","seq":2,"time":1700000000000,"data":{"usage":{"inputTokens":100,"outputTokens":20},"message":{"id":"m1","source":{"model":"deepseek-v4-pro"}}}}`,
		`{"type":"assistant/message","seq":2,"time":1700000000000,"data":{"usage":{"inputTokens":200,"outputTokens":40},"message":{"id":"m1","source":{"model":"deepseek-v4-pro"}}}}`,
	)
	usages, err := parseTranscript(strings.NewReader(text), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].InputTokens != 200 || usages[0].OutputTokens != 40 {
		t.Fatalf("expected the last occurrence to win, got %+v", usages)
	}
}

func TestSourceSkipsUnchangedHistoricalRecords(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DSH_HOME", root)
	first := transcriptText(headerLine, usageOne)
	file := writeSessionBytes(t, root, "proj", "s1", "session.jsonl", []byte(first))

	database, calc := testutil.NewTestDBAndCalc(t)
	var emitted []model.TokenUsage
	sink := source.NewDBSink(database, calc, "local", func(u model.TokenUsage) {
		emitted = append(emitted, u)
	})
	src := New("")

	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(emitted) != 1 {
		t.Fatalf("first scan should emit one record, got %d", len(emitted))
	}

	if err := os.WriteFile(file, []byte(first+usageTwo+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	emitted = nil
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(emitted) != 1 || emitted[0].InputTokens != 200 {
		t.Fatalf("append should emit only the new record, got %+v", emitted)
	}
}

func TestSourceScanCadence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DSH_HOME", root)
	writeSessionBytes(t, root, "proj", "s1", "session.jsonl", []byte(transcriptText(headerLine, usageOne)))

	database, calc := testutil.NewTestDBAndCalc(t)
	sink := testutil.NewTestSink(t, database, calc, "local")
	src := New("")
	src.fullInterval = time.Hour

	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}

	// A new session is not discovered until the full-scan interval elapses.
	writeSessionBytes(t, root, "proj", "s2", "session.jsonl", []byte(transcriptText(
		`{"type":"session","id":"s2","cwd":"/work/demo"}`,
		usageTwo,
	)))
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err := database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 1 {
		t.Fatalf("known-file scan must not discover a new session, got %+v", stats)
	}

	src.lastFull = time.Time{}
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	stats, err = database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 2 {
		t.Fatalf("full scan should discover the new session, got %+v", stats)
	}
}

func TestParseTranscriptFileKeepsCompletePrefixOnTornZstd(t *testing.T) {
	first := compressZstd(t, transcriptText(headerLine, usageOne))
	second := compressZstd(t, transcriptText(usageTwo))
	data := append(append([]byte{}, first...), second...)
	data = data[:len(first)+len(second)/2]

	root := t.TempDir()
	file := writeSessionBytes(t, root, "proj", "s1", "session.v3.jsonl.zstd", data)
	usages, err := parseTranscriptFile(file, true, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].InputTokens != 160 {
		t.Fatalf("expected the complete first frame only, got %+v", usages)
	}
}

func TestSourceScanMissingRootIsNoop(t *testing.T) {
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "missing"))
	database, calc := testutil.NewTestDBAndCalc(t)
	sink := testutil.NewTestSink(t, database, calc, "local")
	if err := New("").Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
}

func TestSourceScanAndScanFileAreSerialized(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DSH_HOME", root)
	file := writeSessionBytes(t, root, "proj", "s1", "session.jsonl", []byte(transcriptText(headerLine, usageOne)))

	database, calc := testutil.NewTestDBAndCalc(t)
	sink := testutil.NewTestSink(t, database, calc, "local")
	src := New("")

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = src.Scan(context.Background(), sink)
		}()
		go func() {
			defer wg.Done()
			_ = src.ScanFile(context.Background(), file, sink)
		}()
	}
	wg.Wait()

	stats, err := database.QueryStatsSince(time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 1 {
		t.Fatalf("concurrent scans must emit one row, got %+v", stats)
	}
}
