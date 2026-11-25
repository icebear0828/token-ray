package db_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

func TestAllOffsets(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.SetOffset("/a.jsonl", 10); err != nil {
		t.Fatal(err)
	}
	if err := database.SetOffset("/b.jsonl", 20); err != nil {
		t.Fatal(err)
	}

	offsets, err := database.AllOffsets()
	if err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 2 || offsets["/a.jsonl"] != 10 || offsets["/b.jsonl"] != 20 {
		t.Fatalf("unexpected offsets: %v", offsets)
	}
}

func TestRunInTxCommitsAndRollsBack(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.RunInTx(func(tx *db.Tx) error {
		return tx.SetOffset("/committed.jsonl", 7)
	}); err != nil {
		t.Fatal(err)
	}
	offset, err := database.GetOffset("/committed.jsonl")
	if err != nil || offset != 7 {
		t.Fatalf("expected committed offset 7, got %d err=%v", offset, err)
	}

	wantErr := errors.New("boom")
	err = database.RunInTx(func(tx *db.Tx) error {
		if err := tx.SetOffset("/rolled-back.jsonl", 9); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	offset, err = database.GetOffset("/rolled-back.jsonl")
	if err != nil || offset != 0 {
		t.Fatalf("expected rollback to discard offset, got %d err=%v", offset, err)
	}
}

func TestStartupExistenceChecks(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	needsBackfill, err := database.HasRecordsNeedingProjectBackfill()
	if err != nil {
		t.Fatal(err)
	}
	if needsBackfill {
		t.Fatal("clean empty DB should not need project backfill")
	}

	hasZeroCost, err := database.HasZeroCostUsageRecords()
	if err != nil {
		t.Fatal(err)
	}
	if hasZeroCost {
		t.Fatal("clean empty DB should not have zero-cost records")
	}

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "token-ray", InputTokens: 100, OutputTokens: 50, UUID: "clean-1"},
		1.25, now, "/Users/c/.claude/projects/token-ray/s.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}

	needsBackfill, err = database.HasRecordsNeedingProjectBackfill()
	if err != nil || needsBackfill {
		t.Fatalf("clean attributed DB should return false, got %v err=%v", needsBackfill, err)
	}
	hasZeroCost, err = database.HasZeroCostUsageRecords()
	if err != nil || hasZeroCost {
		t.Fatalf("fully priced DB should return false, got %v err=%v", hasZeroCost, err)
	}

	// Zero-token $0 row should not trigger HasZeroCostUsageRecords.
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "token-ray", UUID: "zero-tok"},
		0, now, "/Users/c/.claude/projects/token-ray/z.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	hasZeroCost, err = database.HasZeroCostUsageRecords()
	if err != nil || hasZeroCost {
		t.Fatalf("zero-token row should not trigger HasZeroCostUsageRecords, got %v err=%v", hasZeroCost, err)
	}

	// Active Default-project row with file_path triggers HasRecordsNeedingProjectBackfill.
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "Default", InputTokens: 100, OutputTokens: 50, UUID: "needs-proj"},
		1.0, now, "/Users/c/.claude/projects/alpha/s.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	needsBackfill, err = database.HasRecordsNeedingProjectBackfill()
	if err != nil || !needsBackfill {
		t.Fatalf("expected HasRecordsNeedingProjectBackfill=true, got %v err=%v", needsBackfill, err)
	}

	// Active non-zero-token $0 row triggers HasZeroCostUsageRecords.
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "token-ray", InputTokens: 1000, OutputTokens: 100, UUID: "needs-cost"},
		0, now, "/Users/c/.claude/projects/token-ray/c.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	hasZeroCost, err = database.HasZeroCostUsageRecords()
	if err != nil || !hasZeroCost {
		t.Fatalf("expected HasZeroCostUsageRecords=true, got %v err=%v", hasZeroCost, err)
	}
}
