package db_test

import (
	"errors"
	"path/filepath"
	"testing"

	"ai-flight-dashboard/internal/db"
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
