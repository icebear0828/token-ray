package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/testutil"
)

func TestSourceScanEmitsUsage(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(home, ".afd-plugin-scan-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	logFile := filepath.Join(dir, "session.jsonl")
	line := `{"id":"e2e-1","model":"gpt-5.5","ts":"2026-01-02T03:04:05Z","cwd":"/work/demo-proj","usage":{"input":1000000,"cached":0,"output":1000}}` + "\n"
	if err := os.WriteFile(logFile, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	manifest := Manifest{
		SchemaVersion: 1,
		ID:            "demo",
		Name:          "Demo",
		Source:        "Demo",
		Version:       "1.0.0",
		WatchDirs:     []string{dir},
		Fields: FieldMap{
			Model:        "model",
			InputTokens:  "usage.input",
			CachedTokens: "usage.cached",
			OutputTokens: "usage.output",
			Timestamp:    "ts",
			UUID:         "id",
			Project:      "cwd",
		},
	}
	src, err := NewSource(manifest)
	if err != nil {
		t.Fatal(err)
	}

	database, calc := testutil.NewTestDBAndCalc(t)
	sink := source.NewDBSink(database, calc, "test-device", nil)
	if err := src.Scan(context.Background(), sink); err != nil {
		t.Fatalf("scan: %v", err)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 1 {
		t.Fatalf("expected one Demo event, got %+v", stats)
	}
}
