package main

import (
	"token-ray/internal/codexusage"
	"token-ray/internal/db"
	"log"
)

const codexTelemetryBackfillMigrationKey = "migration:codex-telemetry-event-prefix-v1"

func queueCodexTelemetryBackfill(database *db.DB) {
	done, err := database.GetOffset(codexTelemetryBackfillMigrationKey)
	if err != nil {
		log.Printf("Failed to read Codex telemetry backfill migration state: %v", err)
		return
	}
	if done == 1 {
		return
	}
	if _, err := database.ResetOffset(codexusage.OffsetKey); err != nil {
		log.Printf("Failed to queue Codex telemetry backfill: %v", err)
		return
	}
	if err := database.SetOffset(codexTelemetryBackfillMigrationKey, 1); err != nil {
		log.Printf("Failed to mark Codex telemetry backfill migration complete: %v", err)
	}
}
