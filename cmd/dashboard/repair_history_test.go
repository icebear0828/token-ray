package main

import (
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDiscoverGeminiHistoryFilesOnlyReturnsReplayableJSONL(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	geminiDir := filepath.Join(root, ".gemini", "tmp", "wiki", "chats")
	claudeDir := filepath.Join(root, ".claude", "projects", "-Users-c-token")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	geminiA := filepath.Join(geminiDir, "a.jsonl")
	geminiB := filepath.Join(geminiDir, "b.jsonl")
	geminiUsage := `{"id":"abc","timestamp":"2026-05-01T02:44:45.432Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-3.1-pro-preview"}` + "\n"
	for _, path := range []string{
		geminiA,
		geminiB,
	} {
		if err := os.WriteFile(path, []byte(geminiUsage), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(geminiDir, "ignored.txt"),
		filepath.Join(geminiDir, "unparseable.jsonl"),
		filepath.Join(claudeDir, "session.jsonl"),
	} {
		if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := discoverGeminiHistoryFiles([]string{root, filepath.Join(root, ".gemini", "tmp")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{geminiA, geminiB}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected Gemini history files:\nwant: %#v\n got: %#v", want, files)
	}
}
func TestLocalRepairDeviceIDsIncludesCurrentAndLegacyLocalOnce(t *testing.T) {
	if got, want := localRepairDeviceIDs("macbook"), []string{"macbook", "local", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected device IDs: want %#v got %#v", want, got)
	}
	if got, want := localRepairDeviceIDs("local"), []string{"local", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected legacy local device IDs: want %#v got %#v", want, got)
	}
}
func TestRunRepairHistorySupersedesLocalGeminiLegacyRows(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	geminiDir := filepath.Join(root, ".gemini", "tmp", "wiki", "chats")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	geminiFile := filepath.Join(geminiDir, "session.jsonl")
	geminiUsage := `{"timestamp":"2026-05-07T11:13:03.316Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-2.5-pro"}` + "\n"
	if err := os.WriteFile(geminiFile, []byte(geminiUsage), 0644); err != nil {
		t.Fatal(err)
	}

	oldTS := time.Date(2026, 5, 7, 11, 13, 3, 0, time.UTC)
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 800, OutputTokens: 500},
		10.00, oldTS, geminiFile, "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 700, OutputTokens: 400},
		20.00, oldTS, geminiFile, "remote-mac",
	); err != nil {
		t.Fatal(err)
	}

	runRepairHistory(database, calc, "local", []string{root})

	localCost, localIn, localCached, _, localOut, err := database.QueryPeriodStatsAll("local", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if localCost <= 0 || localIn != 1000 || localCached != 250 || localOut != 50 {
		t.Fatalf("expected local stats from repaired active row only, cost=%f input=%d cached=%d output=%d", localCost, localIn, localCached, localOut)
	}

	remoteCost, remoteIn, _, _, remoteOut, err := database.QueryPeriodStatsAll("remote-mac", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if remoteCost != 20.00 || remoteIn != 700 || remoteOut != 400 {
		t.Fatalf("expected remote legacy row to remain active, cost=%f input=%d output=%d", remoteCost, remoteIn, remoteOut)
	}
}
func TestRunRepairHistoryWithNoOptionalSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	runRepairHistory(database, calc, "local", nil)

	for _, source := range []string{"Claude Code", "Gemini CLI", "Codex"} {
		cost, input, cached, cacheCreation, output, err := database.QueryPeriodStatsAll("", source)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 0 || input != 0 || cached != 0 || cacheCreation != 0 || output != 0 {
			t.Fatalf("expected no %s rows, cost=%f input=%d cached=%d cacheCreation=%d output=%d", source, cost, input, cached, cacheCreation, output)
		}
	}
}
