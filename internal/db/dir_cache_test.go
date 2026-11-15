package db_test

import (
	"path/filepath"
	"testing"

	"token-ray/internal/db"
)

func TestDirCache(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Initially empty
	dirs, err := database.ListKnownDirs()
	if err != nil {
		t.Fatalf("ListKnownDirs error: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}

	// Insert two directories
	database.UpsertKnownDir("/home/user/.claude/projects/proj-a")
	database.UpsertKnownDir("/home/user/.claude/projects/proj-b")

	dirs, _ = database.ListKnownDirs()
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}

	// Upsert same dir again — should not duplicate
	database.UpsertKnownDir("/home/user/.claude/projects/proj-a")
	dirs, _ = database.ListKnownDirs()
	if len(dirs) != 2 {
		t.Errorf("expected 2 dirs after upsert, got %d", len(dirs))
	}
}
