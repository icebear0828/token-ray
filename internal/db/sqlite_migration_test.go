package db_test

import (
	"token-ray/internal/db"
	"token-ray/internal/model"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyDatabaseMigratesStatsColumnsAndIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
		CREATE TABLE usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			log_timestamp DATETIME,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			project TEXT DEFAULT 'Default',
			input_tokens INTEGER NOT NULL,
			cached_tokens INTEGER NOT NULL,
			cache_creation_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER NOT NULL,
			cost_usd REAL NOT NULL,
			file_path TEXT DEFAULT '',
			device_id TEXT DEFAULT 'local',
			uuid TEXT
		);
		CREATE TABLE scan_offsets (
			file_path TEXT PRIMARY KEY,
			byte_offset INTEGER NOT NULL
		);
		INSERT INTO usage_records (log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id)
		VALUES ('2026-05-07T11:13:03Z', 'Codex', 'gpt-5.5', 'legacy-win', 1000, 100, 0, 200, 1.25, 'C:\Users\c\.codex\logs_2.sqlite', 'windows-box');
	`)
	if err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cost, input, cached, _, output, err := database.QueryPeriodStatsAll("windows-box", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 1.25 || input != 1000 || cached != 100 || output != 200 {
		t.Fatalf("expected migrated legacy Codex row in stats, got cost=%f input=%d cached=%d output=%d", cost, input, cached, output)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "windows-box", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Source != "Codex" {
		t.Fatalf("expected all-time Codex stats after migration, got %+v", stats)
	}

	check, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	for _, indexName := range []string{
		"idx_usage_active_source_log_jd",
		"idx_usage_active_device_source_log_jd",
		"idx_usage_active_source_model",
		"idx_usage_active_source_project",
	} {
		var found string
		err := check.QueryRow("SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", indexName).Scan(&found)
		if err != nil {
			t.Fatalf("expected migrated index %s: %v", indexName, err)
		}
	}
}
func TestExistingDatabaseMigrationAddsUpdatedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		CREATE TABLE usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			log_timestamp DATETIME,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			project TEXT DEFAULT 'Default',
			input_tokens INTEGER NOT NULL,
			cached_tokens INTEGER NOT NULL,
			cache_creation_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER NOT NULL,
			cost_usd REAL NOT NULL,
			file_path TEXT DEFAULT '',
			device_id TEXT DEFAULT 'local',
			uuid TEXT,
			superseded INTEGER DEFAULT 0
		);
		CREATE TABLE scan_offsets (file_path TEXT PRIMARY KEY, byte_offset INTEGER NOT NULL);
		CREATE TABLE known_dirs (dir_path TEXT PRIMARY KEY, last_seen DATETIME DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE device_aliases (device_id TEXT PRIMARY KEY, display_name TEXT NOT NULL);
		INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id)
		VALUES ('2026-01-01T00:00:00Z', 'Claude Code', 'claude-opus-4-7', 100, 0, 0, 10, 1.00, '/old.jsonl', 'mac');
	`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 200, OutputTokens: 20, UUID: "codex:1"},
		2.00, time.Now().UTC(), "/codex.sqlite", "mac",
	); err != nil {
		t.Fatal(err)
	}
	records, err := database.QuerySyncRecordsSince(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected migrated legacy and new sync records, got %+v", records)
	}
	for _, r := range records {
		if r.UpdatedAt.IsZero() {
			t.Fatalf("expected migrated updated_at, got %+v", records)
		}
	}
}
