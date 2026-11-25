package db

import (
	"ai-flight-dashboard/internal/model"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// QueryDevices returns a list of unique devices.
func (d *DB) QueryDevices() ([]string, error) {
	rows, err := d.conn.Query("SELECT DISTINCT device_id FROM usage_records WHERE " + activeUsagePredicate + " ORDER BY device_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		devices = append(devices, id)
	}
	return devices, rows.Err()
}

// QueryDeviceSummaries returns active device aggregates plus manual aliases.
func (d *DB) QueryDeviceSummaries() ([]model.DeviceSummary, error) {
	query := `
		WITH device_ids AS (
			SELECT DISTINCT device_id FROM usage_records WHERE ` + activeUsagePredicate + `
			UNION
			SELECT device_id FROM device_aliases
		),
		agg AS (
			SELECT device_id,
				COUNT(*) AS events,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cost_usd), 0) AS total_cost,
				MIN(log_timestamp) AS first_seen,
				MAX(log_timestamp) AS last_seen
			FROM usage_records
			WHERE ` + activeUsagePredicate + `
			GROUP BY device_id
		)
		SELECT device_ids.device_id,
			COALESCE(device_aliases.display_name, device_ids.device_id) AS display_name,
			COALESCE(agg.events, 0),
			COALESCE(agg.input_tokens, 0),
			COALESCE(agg.cached_tokens, 0),
			COALESCE(agg.cache_creation_tokens, 0),
			COALESCE(agg.output_tokens, 0),
			COALESCE(agg.total_cost, 0),
			agg.first_seen,
			agg.last_seen
		FROM device_ids
		LEFT JOIN agg ON agg.device_id = device_ids.device_id
		LEFT JOIN device_aliases ON device_aliases.device_id = device_ids.device_id
		ORDER BY COALESCE(agg.total_cost, 0) DESC, device_ids.device_id
	`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := make([]model.DeviceSummary, 0, 8)
	for rows.Next() {
		var summary model.DeviceSummary
		var firstSeen sql.NullString
		var lastSeen sql.NullString
		if err := rows.Scan(
			&summary.ID,
			&summary.DisplayName,
			&summary.Events,
			&summary.InputTokens,
			&summary.CachedTokens,
			&summary.CacheCreationTokens,
			&summary.OutputTokens,
			&summary.TotalCost,
			&firstSeen,
			&lastSeen,
		); err != nil {
			return nil, err
		}
		if firstSeen.Valid {
			if ts, err := parseLogTimestamp(firstSeen.String); err == nil {
				summary.FirstSeen = ts
			}
		}
		if lastSeen.Valid {
			if ts, err := parseLogTimestamp(lastSeen.String); err == nil {
				summary.LastSeen = ts
			}
		}
		devices = append(devices, summary)
	}
	return devices, rows.Err()
}

// SupersedeDevice hides all active rows for a device ID without deleting them.
func (d *DB) SupersedeDevice(deviceID string) (int64, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return 0, errors.New("device id is required")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	result, err := tx.Exec(
		"UPDATE usage_records SET superseded = 1, updated_at = ? WHERE device_id = ? AND "+activeUsagePredicate,
		formatLogTimestamp(time.Now().UTC()), deviceID,
	)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	if _, err := tx.Exec("DELETE FROM device_aliases WHERE device_id = ?", deviceID); err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

// SetDeviceAlias sets or updates the display name for a device ID.
func (d *DB) SetDeviceAlias(deviceID, displayName string) error {
	deviceID = strings.TrimSpace(deviceID)
	displayName = strings.TrimSpace(displayName)
	if deviceID == "" || displayName == "" {
		return errors.New("device id and display name are required")
	}
	query := `INSERT INTO device_aliases (device_id, display_name) VALUES (?, ?)
	ON CONFLICT(device_id) DO UPDATE SET display_name = excluded.display_name`
	_, err := d.conn.Exec(query, deviceID, displayName)
	return err
}

// DeleteDeviceAlias removes a display name override for a device ID.
func (d *DB) DeleteDeviceAlias(deviceID string) (int64, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return 0, errors.New("device id is required")
	}
	result, err := d.conn.Exec("DELETE FROM device_aliases WHERE device_id = ?", deviceID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetDeviceAliases returns a map of deviceID to displayName.
func (d *DB) GetDeviceAliases() (map[string]string, error) {
	rows, err := d.conn.Query("SELECT device_id, display_name FROM device_aliases")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			aliases[id] = name
		}
	}
	return aliases, rows.Err()
}

// RawExec executes a raw SQL statement. Exposed for testing only.
