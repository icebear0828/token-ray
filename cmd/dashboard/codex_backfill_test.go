package main

import (
	"token-ray/internal/codexusage"
	"token-ray/internal/testutil"
	"testing"
)

func TestQueueCodexTelemetryBackfillResetsOffsetOnce(t *testing.T) {
	database := testutil.NewTestDB(t)
	if err := database.SetOffset(codexusage.OffsetKey, 12345); err != nil {
		t.Fatal(err)
	}

	queueCodexTelemetryBackfill(database)

	offset, err := database.GetOffset(codexusage.OffsetKey)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatalf("expected Codex offset reset to 0, got %d", offset)
	}
	done, err := database.GetOffset(codexTelemetryBackfillMigrationKey)
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 {
		t.Fatalf("expected Codex backfill migration marked done, got %d", done)
	}

	if err := database.SetOffset(codexusage.OffsetKey, 67890); err != nil {
		t.Fatal(err)
	}
	queueCodexTelemetryBackfill(database)
	offset, err = database.GetOffset(codexusage.OffsetKey)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 67890 {
		t.Fatalf("expected Codex offset unchanged after migration is done, got %d", offset)
	}
}
