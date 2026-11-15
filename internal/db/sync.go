package db

import (
	"token-ray/internal/model"
	"database/sql"
	"time"
)

// UpsertSyncRecord applies a LAN synchronization record, including tombstone-like
// superseded markers for legacy rows that do not have UUIDs.
func (d *DB) UpsertSyncRecord(r model.SyncRecord) error {
	deviceID := r.DeviceID
	if deviceID == "" {
		deviceID = r.TokenUsage.DeviceID
	}
	u := r.TokenUsage
	u.DeviceID = deviceID
	updatedAt := r.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if r.Superseded {
		if u.UUID != "" {
			return d.insertUsageWithTimeUpdatedAt(u, r.CostUSD, u.Timestamp, r.FilePath, deviceID, updatedAt, true)
		}
		return d.supersedeOrInsertLegacySyncRecord(u, r.CostUSD, r.FilePath, deviceID, updatedAt)
	}
	return d.insertUsageWithTimeUpdatedAt(u, r.CostUSD, u.Timestamp, r.FilePath, deviceID, updatedAt, false)
}
func (d *DB) supersedeOrInsertLegacySyncRecord(u model.TokenUsage, cost float64, filePath string, deviceID string, updatedAtTS time.Time) error {
	logTimestamp := formatLogTimestamp(u.Timestamp)
	updatedAt := formatLogTimestamp(updatedAtTS)
	result, err := d.conn.Exec(`UPDATE usage_records SET superseded = 1, updated_at = ?
		WHERE log_timestamp = ? AND source = ? AND model = ? AND input_tokens = ? AND cached_tokens = ?
			AND cache_creation_tokens = ? AND output_tokens = ? AND file_path = ?
			AND device_id = ? AND (uuid IS NULL OR uuid = '')`,
		updatedAt, logTimestamp, u.Source, u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, filePath, deviceID,
	)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed > 0 {
		return nil
	}
	return d.insertUsageWithTimeUpdatedAt(u, cost, u.Timestamp, filePath, deviceID, updatedAtTS, true)
}

// QuerySyncRecordsSince returns raw records for P2P LAN DB synchronization.
// The cursor is based on updated_at, not log_timestamp, so historical repairs
// and superseded markers propagate even when the underlying usage happened long ago.
func (d *DB) QuerySyncRecordsSince(since time.Time) ([]model.SyncRecord, error) {
	page, err := d.QuerySyncRecordsPage(since, 0, 0)
	if err != nil {
		return nil, err
	}
	return page.Records, nil
}

// QuerySyncRecordsPage returns a page of synchronization records. The cursor is
// (updated_at, id), so rows with identical updated_at values cannot be skipped.
// QuerySyncRecordsPage returns a page of synchronization records. The cursor is
// (updated_at, id), so rows with identical updated_at values cannot be skipped.
func (d *DB) QuerySyncRecordsPage(since time.Time, afterID int64, limit int) (model.SyncPullResponse, error) {
	return d.querySyncRecordsPage(since, afterID, limit, "")
}

// QuerySyncRecordsPageForDevice returns a page scoped to one device ID.
// QuerySyncRecordsPageForDevice returns a page scoped to one device ID.
func (d *DB) QuerySyncRecordsPageForDevice(since time.Time, afterID int64, limit int, deviceID string) (model.SyncPullResponse, error) {
	return d.querySyncRecordsPage(since, afterID, limit, deviceID)
}
func (d *DB) querySyncRecordsPage(since time.Time, afterID int64, limit int, deviceID string) (model.SyncPullResponse, error) {
	query := `SELECT uuid, log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, COALESCE(superseded, 0), COALESCE(updated_at, timestamp, log_timestamp), id
		FROM usage_records WHERE `
	args := []interface{}{formatLogTimestamp(since)}
	if afterID > 0 {
		query += `(julianday(COALESCE(updated_at, timestamp, log_timestamp)) > julianday(?)
			OR (julianday(COALESCE(updated_at, timestamp, log_timestamp)) = julianday(?) AND id > ?))`
		args = append(args, formatLogTimestamp(since), afterID)
	} else {
		query += `julianday(COALESCE(updated_at, timestamp, log_timestamp)) >= julianday(?)`
	}
	if deviceID != "" {
		query += ` AND device_id = ?`
		args = append(args, deviceID)
	}
	query += ` ORDER BY julianday(COALESCE(updated_at, timestamp, log_timestamp)), id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return model.SyncPullResponse{}, err
	}
	defer rows.Close()

	var records []model.SyncRecord
	var lastUpdatedAt time.Time
	var lastID int64
	hasMore := false
	for rows.Next() {
		var r model.SyncRecord
		var rowID int64
		var logTsStr string
		var updatedAtStr string
		var uuidStr sql.NullString
		var superseded int

		if err := rows.Scan(&uuidStr, &logTsStr, &r.Source, &r.Model, &r.Project, &r.InputTokens, &r.CachedTokens, &r.CacheCreationTokens, &r.OutputTokens, &r.CostUSD, &r.FilePath, &r.DeviceID, &superseded, &updatedAtStr, &rowID); err != nil {
			return model.SyncPullResponse{}, err
		}
		if limit > 0 && len(records) >= limit {
			hasMore = true
			break
		}

		r.UUID = uuidStr.String
		if t, err := parseLogTimestamp(logTsStr); err == nil {
			r.Timestamp = t
		}
		if t, err := parseLogTimestamp(updatedAtStr); err == nil {
			r.UpdatedAt = t
		}
		r.Superseded = superseded != 0
		records = append(records, r)
		lastUpdatedAt = r.UpdatedAt
		lastID = rowID
	}
	if err := rows.Err(); err != nil {
		return model.SyncPullResponse{}, err
	}
	return model.SyncPullResponse{
		Records:       records,
		NextUpdatedAt: lastUpdatedAt,
		NextAfterID:   lastID,
		HasMore:       hasMore,
	}, nil
}
