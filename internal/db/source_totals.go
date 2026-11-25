package db

import (
	"database/sql"
	"time"
)

func ensureSourceTotalsSummary(conn *sql.DB) error {
	if err := execMigration(conn, "CREATE INDEX IF NOT EXISTS idx_usage_source_totals_source ON usage_source_totals(source)", false); err != nil {
		return err
	}

	var done int64
	err := conn.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", sourceTotalsSummaryMigrationKey).Scan(&done)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	recreate := err == sql.ErrNoRows || done != 1
	if err := ensureSourceTotalsSummaryTriggers(conn, recreate); err != nil {
		return err
	}
	if !recreate {
		return nil
	}
	if err := rebuildSourceTotalsSummary(conn); err != nil {
		return err
	}
	_, err = conn.Exec(`INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, 1)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`, sourceTotalsSummaryMigrationKey)
	return err
}
func ensureSourceTotalsSummaryTriggers(conn *sql.DB, recreate bool) error {
	if recreate {
		for _, stmt := range []string{
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_insert",
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_delete",
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_update_remove",
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_update_add",
		} {
			if err := execMigration(conn, stmt, false); err != nil {
				return err
			}
		}
	}

	for _, stmt := range []string{
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_insert
		AFTER INSERT ON usage_records
		WHEN COALESCE(NEW.superseded, 0) = 0
		BEGIN
			INSERT INTO usage_source_totals (
				device_id, source, events, input_tokens, cached_tokens,
				cache_creation_tokens, output_tokens, cost_usd
			)
			VALUES (
				COALESCE(NEW.device_id, 'local'), NEW.source, 1,
				COALESCE(NEW.input_tokens, 0), COALESCE(NEW.cached_tokens, 0),
				COALESCE(NEW.cache_creation_tokens, 0), COALESCE(NEW.output_tokens, 0),
				COALESCE(NEW.cost_usd, 0)
			)
			ON CONFLICT(device_id, source) DO UPDATE SET
				events = usage_source_totals.events + excluded.events,
				input_tokens = usage_source_totals.input_tokens + excluded.input_tokens,
				cached_tokens = usage_source_totals.cached_tokens + excluded.cached_tokens,
				cache_creation_tokens = usage_source_totals.cache_creation_tokens + excluded.cache_creation_tokens,
				output_tokens = usage_source_totals.output_tokens + excluded.output_tokens,
				cost_usd = usage_source_totals.cost_usd + excluded.cost_usd;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_delete
		AFTER DELETE ON usage_records
		WHEN COALESCE(OLD.superseded, 0) = 0
		BEGIN
			UPDATE usage_source_totals SET
				events = events - 1,
				input_tokens = input_tokens - COALESCE(OLD.input_tokens, 0),
				cached_tokens = cached_tokens - COALESCE(OLD.cached_tokens, 0),
				cache_creation_tokens = cache_creation_tokens - COALESCE(OLD.cache_creation_tokens, 0),
				output_tokens = output_tokens - COALESCE(OLD.output_tokens, 0),
				cost_usd = cost_usd - COALESCE(OLD.cost_usd, 0)
			WHERE device_id = COALESCE(OLD.device_id, 'local') AND source = OLD.source;
			DELETE FROM usage_source_totals WHERE events <= 0;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_update_remove
		AFTER UPDATE ON usage_records
		WHEN COALESCE(OLD.superseded, 0) = 0
			AND (
				COALESCE(NEW.superseded, 0) != 0
				OR COALESCE(OLD.device_id, 'local') IS NOT COALESCE(NEW.device_id, 'local')
				OR OLD.source IS NOT NEW.source
				OR COALESCE(OLD.input_tokens, 0) IS NOT COALESCE(NEW.input_tokens, 0)
				OR COALESCE(OLD.cached_tokens, 0) IS NOT COALESCE(NEW.cached_tokens, 0)
				OR COALESCE(OLD.cache_creation_tokens, 0) IS NOT COALESCE(NEW.cache_creation_tokens, 0)
				OR COALESCE(OLD.output_tokens, 0) IS NOT COALESCE(NEW.output_tokens, 0)
				OR COALESCE(OLD.cost_usd, 0) IS NOT COALESCE(NEW.cost_usd, 0)
			)
		BEGIN
			UPDATE usage_source_totals SET
				events = events - 1,
				input_tokens = input_tokens - COALESCE(OLD.input_tokens, 0),
				cached_tokens = cached_tokens - COALESCE(OLD.cached_tokens, 0),
				cache_creation_tokens = cache_creation_tokens - COALESCE(OLD.cache_creation_tokens, 0),
				output_tokens = output_tokens - COALESCE(OLD.output_tokens, 0),
				cost_usd = cost_usd - COALESCE(OLD.cost_usd, 0)
			WHERE device_id = COALESCE(OLD.device_id, 'local') AND source = OLD.source;
			DELETE FROM usage_source_totals WHERE events <= 0;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_update_add
		AFTER UPDATE ON usage_records
		WHEN COALESCE(NEW.superseded, 0) = 0
			AND (
				COALESCE(OLD.superseded, 0) != 0
				OR COALESCE(OLD.device_id, 'local') IS NOT COALESCE(NEW.device_id, 'local')
				OR OLD.source IS NOT NEW.source
				OR COALESCE(OLD.input_tokens, 0) IS NOT COALESCE(NEW.input_tokens, 0)
				OR COALESCE(OLD.cached_tokens, 0) IS NOT COALESCE(NEW.cached_tokens, 0)
				OR COALESCE(OLD.cache_creation_tokens, 0) IS NOT COALESCE(NEW.cache_creation_tokens, 0)
				OR COALESCE(OLD.output_tokens, 0) IS NOT COALESCE(NEW.output_tokens, 0)
				OR COALESCE(OLD.cost_usd, 0) IS NOT COALESCE(NEW.cost_usd, 0)
			)
		BEGIN
			INSERT INTO usage_source_totals (
				device_id, source, events, input_tokens, cached_tokens,
				cache_creation_tokens, output_tokens, cost_usd
			)
			VALUES (
				COALESCE(NEW.device_id, 'local'), NEW.source, 1,
				COALESCE(NEW.input_tokens, 0), COALESCE(NEW.cached_tokens, 0),
				COALESCE(NEW.cache_creation_tokens, 0), COALESCE(NEW.output_tokens, 0),
				COALESCE(NEW.cost_usd, 0)
			)
			ON CONFLICT(device_id, source) DO UPDATE SET
				events = usage_source_totals.events + excluded.events,
				input_tokens = usage_source_totals.input_tokens + excluded.input_tokens,
				cached_tokens = usage_source_totals.cached_tokens + excluded.cached_tokens,
				cache_creation_tokens = usage_source_totals.cache_creation_tokens + excluded.cache_creation_tokens,
				output_tokens = usage_source_totals.output_tokens + excluded.output_tokens,
				cost_usd = usage_source_totals.cost_usd + excluded.cost_usd;
		END`,
	} {
		if err := execMigration(conn, stmt, false); err != nil {
			return err
		}
	}
	return nil
}

// RebuildSourceTotalsSummary rebuilds the materialized all-time source totals
// from usage_records. The raw usage table remains the source of truth.
// RebuildSourceTotalsSummary rebuilds the materialized all-time source totals
// from usage_records. The raw usage table remains the source of truth.
func (d *DB) RebuildSourceTotalsSummary() error {
	return rebuildSourceTotalsSummary(d.conn)
}
func rebuildSourceTotalsSummary(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM usage_source_totals"); err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO usage_source_totals (
			device_id, source, events, input_tokens, cached_tokens,
			cache_creation_tokens, output_tokens, cost_usd
		)
		SELECT
			COALESCE(device_id, 'local'), source, COUNT(*),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(cache_creation_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cost_usd), 0)
		FROM usage_records
		WHERE ` + activeUsagePredicate + `
		GROUP BY COALESCE(device_id, 'local'), source
	`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// InsertUsage inserts a usage record with current timestamp (for live watcher).
// QuerySourceTotalsSince returns per-source aggregate stats without model/project details.
func (d *DB) QuerySourceTotalsSince(since time.Time, deviceID string, source string) ([]SourceTotalStat, error) {
	if since.IsZero() {
		return d.QuerySourceTotalsSummary(deviceID, source)
	}

	query := `
		SELECT source, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
	`
	var args []interface{}

	if !since.IsZero() {
		query += " AND julianday(log_timestamp) >= julianday(?)"
		args = append(args, formatLogTimestamp(since))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += `
		GROUP BY source
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make([]SourceTotalStat, 0, 8)
	for rows.Next() {
		var s SourceTotalStat
		if err := rows.Scan(&s.Source, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QuerySourceTotalsSummary returns all-time per-source totals from the maintained summary table.
// QuerySourceTotalsSummary returns all-time per-source totals from the maintained summary table.
func (d *DB) QuerySourceTotalsSummary(deviceID string, source string) ([]SourceTotalStat, error) {
	query := `
		SELECT source, COALESCE(SUM(events), 0) as events,
			COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cost_usd), 0)
		FROM usage_source_totals
		WHERE 1 = 1
	`
	var args []interface{}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += `
		GROUP BY source
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make([]SourceTotalStat, 0, 8)
	for rows.Next() {
		var s SourceTotalStat
		if err := rows.Scan(&s.Source, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QueryProjectStatsSince returns per-project stats since the given time.
