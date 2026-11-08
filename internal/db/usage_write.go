package db

import (
	"ai-flight-dashboard/internal/model"
	"database/sql"
	"errors"
	"math"
	"time"
)

type UsageCostCalculator func(model string, inputTokens int, cachedTokens int, cacheCreationTokens int, outputTokens int) (float64, error)

// sqlExecer is the subset of *sql.DB and *sql.Tx used by usage writes, so the
// same statements can run either directly or inside a transaction.
type sqlExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// InsertUsage inserts a usage record with current timestamp (for live watcher).
func (d *DB) InsertUsage(u model.TokenUsage, cost float64, deviceID string) error {
	ts := time.Now().UTC()
	if !u.Timestamp.IsZero() {
		ts = u.Timestamp
	}
	return d.InsertUsageWithTime(u, cost, ts, "", deviceID)
}

// InsertUsageWithTime inserts a usage record with an explicit log timestamp.
// If UUID is present, it will UPSERT (overwrite older states of the same generation).
// If UUID is empty, duplicate records are silently ignored.
// InsertUsageWithTime inserts a usage record with an explicit log timestamp.
// If UUID is present, it will UPSERT (overwrite older states of the same generation).
// If UUID is empty, duplicate records are silently ignored.
func (d *DB) InsertUsageWithTime(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string) error {
	return d.insertUsageWithTime(u, cost, logTS, filePath, deviceID, false)
}

func (d *DB) RecalculateUsageCosts(calculate UsageCostCalculator) (int64, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT id, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd
		FROM usage_records
	`)
	if err != nil {
		return 0, err
	}

	type costUpdate struct {
		id   int64
		cost float64
	}
	var updates []costUpdate
	for rows.Next() {
		var id int64
		var modelName string
		var inputTokens, cachedTokens, cacheCreationTokens, outputTokens int
		var existingCost float64
		if err := rows.Scan(&id, &modelName, &inputTokens, &cachedTokens, &cacheCreationTokens, &outputTokens, &existingCost); err != nil {
			rows.Close()
			return 0, err
		}
		cost, err := calculate(modelName, inputTokens, cachedTokens, cacheCreationTokens, outputTokens)
		if err != nil {
			rows.Close()
			return 0, err
		}
		if math.Abs(existingCost-cost) <= 0.000000001 {
			continue
		}
		updates = append(updates, costUpdate{id: id, cost: cost})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare("UPDATE usage_records SET cost_usd = ?, updated_at = ? WHERE id = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	updatedAt := formatLogTimestamp(time.Now().UTC())
	var changed int64
	for _, update := range updates {
		result, err := stmt.Exec(update.cost, updatedAt, update.id)
		if err != nil {
			return changed, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return changed, err
		}
		changed += n
	}

	if err := tx.Commit(); err != nil {
		return changed, err
	}
	return changed, nil
}
func (d *DB) insertUsageWithTime(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string, superseded bool) error {
	return insertUsageWithTimeExec(d.conn, u, cost, logTS, filePath, deviceID, superseded)
}
func (d *DB) insertUsageWithTimeUpdatedAt(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string, updatedAtTS time.Time, superseded bool) error {
	return insertUsageWithTimeUpdatedAtExec(d.conn, u, cost, logTS, filePath, deviceID, updatedAtTS, superseded)
}

func insertUsageWithTimeExec(e sqlExecer, u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string, superseded bool) error {
	return insertUsageWithTimeUpdatedAtExec(e, u, cost, logTS, filePath, deviceID, time.Now().UTC(), superseded)
}
func insertUsageWithTimeUpdatedAtExec(e sqlExecer, u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string, updatedAtTS time.Time, superseded bool) error {
	if updatedAtTS.IsZero() {
		updatedAtTS = time.Now().UTC()
	}
	logTimestamp := formatLogTimestamp(logTS)
	updatedAt := formatLogTimestamp(updatedAtTS)
	supersededValue := 0
	if superseded {
		supersededValue = 1
	}
	if u.UUID != "" {
		query := `
		INSERT INTO usage_records (uuid, log_timestamp, updated_at, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id, uuid) WHERE uuid IS NOT NULL AND uuid != '' DO UPDATE SET
			source = excluded.source,
			model = excluded.model,
			log_timestamp = excluded.log_timestamp,
			updated_at = excluded.updated_at,
			project = excluded.project,
			input_tokens = excluded.input_tokens,
			cached_tokens = excluded.cached_tokens,
			cache_creation_tokens = excluded.cache_creation_tokens,
			output_tokens = excluded.output_tokens,
			cost_usd = excluded.cost_usd,
			file_path = CASE
				WHEN excluded.file_path != '' THEN excluded.file_path
				ELSE usage_records.file_path
			END,
			device_id = excluded.device_id,
			superseded = excluded.superseded
		WHERE julianday(excluded.updated_at) >= julianday(COALESCE(usage_records.updated_at, usage_records.timestamp, usage_records.log_timestamp))
		`
		_, err := e.Exec(query, u.UUID, logTimestamp, updatedAt, u.Source, u.Model, u.Project, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, cost, filePath, deviceID, supersededValue)
		return err
	}

	if !superseded {
		newerSuperseded, err := hasNewerSupersededLegacyRecordExec(e, u, logTimestamp, filePath, deviceID, updatedAt)
		if err != nil {
			return err
		}
		if newerSuperseded {
			return nil
		}
	}

	query := `
	INSERT OR IGNORE INTO usage_records (log_timestamp, updated_at, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := e.Exec(query, logTimestamp, updatedAt, u.Source, u.Model, u.Project, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, cost, filePath, deviceID, supersededValue)
	return err
}

func hasNewerSupersededLegacyRecordExec(e sqlExecer, u model.TokenUsage, logTimestamp string, filePath string, deviceID string, updatedAt string) (bool, error) {
	var exists int
	err := e.QueryRow(`
		SELECT 1
		FROM usage_records
		WHERE log_timestamp = ?
			AND source = ?
			AND model = ?
			AND input_tokens = ?
			AND cached_tokens = ?
			AND cache_creation_tokens = ?
			AND output_tokens = ?
			AND file_path = ?
			AND device_id = ?
			AND (uuid IS NULL OR uuid = '')
			AND COALESCE(superseded, 0) != 0
			AND julianday(COALESCE(updated_at, timestamp, log_timestamp)) >= julianday(?)
		LIMIT 1
	`, logTimestamp, u.Source, u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, filePath, deviceID, updatedAt).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

// UpsertSyncRecord applies a LAN synchronization record, including tombstone-like
// superseded markers for legacy rows that do not have UUIDs.
// SupersedeLegacyUsageBySourceFilePathsAndDevices hides legacy no-UUID rows
// matching exact source, file path, and device IDs. Empty path/device lists are
// no-ops. Rows are retained for audit and recovery.
func (d *DB) SupersedeLegacyUsageBySourceFilePathsAndDevices(source string, filePaths []string, deviceIDs []string) (int64, error) {
	if source == "" || len(filePaths) == 0 || len(deviceIDs) == 0 {
		return 0, nil
	}

	seen := make(map[string]struct{})
	var changed int64
	for _, filePath := range filePaths {
		if filePath == "" {
			continue
		}
		for _, deviceID := range deviceIDs {
			key := filePath + "\x00" + deviceID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result, err := d.conn.Exec("UPDATE usage_records SET superseded = 1, updated_at = ? WHERE source = ? AND file_path = ? AND device_id = ? AND (uuid IS NULL OR uuid = '') AND COALESCE(superseded, 0) = 0", formatLogTimestamp(time.Now().UTC()), source, filePath, deviceID)
			if err != nil {
				return changed, err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return changed, err
			}
			changed += n
		}
	}
	return changed, nil
}

// SupersedeUsageBySourceFilePathDeviceUUIDPrefix hides active rows from a
// superseded source that used different UUIDs for the same underlying usage.
// SupersedeUsageBySourceFilePathDeviceUUIDPrefix hides active rows from a
// superseded source that used different UUIDs for the same underlying usage.
func (d *DB) SupersedeUsageBySourceFilePathDeviceUUIDPrefix(source string, filePath string, deviceID string, uuidPrefix string) (int64, error) {
	if source == "" || filePath == "" || deviceID == "" || uuidPrefix == "" {
		return 0, nil
	}
	result, err := d.conn.Exec(
		"UPDATE usage_records SET superseded = 1, updated_at = ? WHERE source = ? AND file_path = ? AND device_id = ? AND uuid LIKE ? AND COALESCE(superseded, 0) = 0",
		formatLogTimestamp(time.Now().UTC()), source, filePath, deviceID, uuidPrefix+"%",
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// QueryPeriodStatsSince returns total cost and token breakdown since the given time.
// source filters by source column (e.g. "Claude Code", "Gemini CLI"); empty means all.
// DeduplicateExisting removes duplicate usage records, keeping the one with the lowest rowid.
// Returns the number of rows removed.
func (d *DB) DeduplicateExisting() (int64, error) {
	query := `
	DELETE FROM usage_records WHERE id NOT IN (
		SELECT MIN(id) FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
		GROUP BY log_timestamp, file_path, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, device_id
	) AND (uuid IS NULL OR uuid = '') AND COALESCE(superseded, 0) = 0`
	result, err := d.conn.Exec(query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SetDeviceAlias sets or updates the display name for a device ID.
// RawExec executes a raw SQL statement. Exposed for testing only.
func (d *DB) RawExec(query string, args ...interface{}) error {
	_, err := d.conn.Exec(query, args...)
	return err
}
