package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const logTimestampLayout = "2006-01-02T15:04:05.000000000Z"
const activeUsagePredicate = "COALESCE(superseded, 0) = 0"
const sourceTotalsSummaryMigrationKey = "migration:source-totals-summary-v2"

var errInvalidLogTimestamp = errors.New("invalid log timestamp")

func initSchema(conn *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		log_timestamp DATETIME,
		source TEXT NOT NULL,
		model TEXT NOT NULL,
		project TEXT DEFAULT 'Default',
		input_tokens INTEGER NOT NULL,
		cached_tokens INTEGER NOT NULL,
		cache_creation_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		timed_output_tokens INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL,
		file_path TEXT DEFAULT '',
		device_id TEXT DEFAULT 'local',
		uuid TEXT,
		superseded INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS scan_offsets (
		file_path TEXT PRIMARY KEY,
		byte_offset INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS known_dirs (
		dir_path TEXT PRIMARY KEY,
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS device_aliases (
		device_id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS usage_source_totals (
		device_id TEXT NOT NULL,
		source TEXT NOT NULL,
		events INTEGER NOT NULL,
		input_tokens INTEGER NOT NULL,
		cached_tokens INTEGER NOT NULL,
		cache_creation_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cost_usd REAL NOT NULL,
		PRIMARY KEY (device_id, source)
	);
	`
	_, err := conn.Exec(schema)
	if err != nil {
		return err
	}

	// Lightweight migrations for existing tables. Index creation intentionally
	// runs after ALTER TABLE so old databases missing newer columns still open.
	for _, stmt := range []string{
		"ALTER TABLE usage_records ADD COLUMN device_id TEXT DEFAULT 'local'",
		"ALTER TABLE usage_records ADD COLUMN cache_creation_tokens INTEGER DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN uuid TEXT",
		"ALTER TABLE usage_records ADD COLUMN project TEXT DEFAULT 'Default'",
		"ALTER TABLE usage_records ADD COLUMN superseded INTEGER DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN updated_at DATETIME",
		"ALTER TABLE usage_records ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN timed_output_tokens INTEGER NOT NULL DEFAULT 0",
	} {
		if err := execMigration(conn, stmt, true); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		"UPDATE usage_records SET updated_at = COALESCE(timestamp, log_timestamp, CURRENT_TIMESTAMP) WHERE updated_at IS NULL OR updated_at = ''",
		"DROP INDEX IF EXISTS idx_usage_uuid",
		"CREATE INDEX IF NOT EXISTS idx_usage_log_ts ON usage_records(log_timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_usage_device ON usage_records(device_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_device_uuid ON usage_records(device_id, uuid) WHERE uuid IS NOT NULL AND uuid != ''",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_log_ts ON usage_records(source, log_timestamp) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_device_source_log_ts ON usage_records(device_id, source, log_timestamp) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_log_jd ON usage_records(source, julianday(log_timestamp)) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_device_source_log_jd ON usage_records(device_id, source, julianday(log_timestamp)) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_model ON usage_records(source, model) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_project ON usage_records(source, project) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_updated_at ON usage_records(julianday(COALESCE(updated_at, timestamp, log_timestamp)))",
	} {
		if err := execMigration(conn, stmt, false); err != nil {
			return err
		}
	}
	if err := ensurePartialDedupIndex(conn); err != nil {
		return err
	}
	if err := ensureSourceTotalsSummary(conn); err != nil {
		return err
	}

	return nil
}
func execMigration(conn *sql.DB, stmt string, ignoreDuplicateColumn bool) error {
	_, err := conn.Exec(stmt)
	if err == nil {
		return nil
	}
	if ignoreDuplicateColumn && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}
func ensurePartialDedupIndex(conn *sql.DB) error {
	const migrationKey = "migration:partial-dedup-index-v2"
	var done int64
	err := conn.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", migrationKey).Scan(&done)
	if err == nil && done == 1 {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := conn.Exec("DROP INDEX IF EXISTS idx_usage_dedup"); err != nil {
		return err
	}
	if _, err := conn.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(log_timestamp, file_path, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, device_id) WHERE (uuid IS NULL OR uuid = '') AND COALESCE(superseded, 0) = 0"); err != nil {
		return err
	}
	_, err = conn.Exec(`INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, 1)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`, migrationKey)
	return err
}
func formatLogTimestamp(ts time.Time) string {
	return ts.UTC().Format(logTimestampLayout)
}
func parseLogTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{
		logTimestampLayout,
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, errInvalidLogTimestamp
}

// GetOffset returns the last scanned byte offset for a file. Returns 0 if not found.
