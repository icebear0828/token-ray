package db

import (
	"ai-flight-dashboard/internal/model"
	"database/sql"
	"strings"
	"time"
)

// QueryPeriodStatsSince returns total cost and token breakdown since the given time.
// source filters by source column (e.g. "Claude Code", "Gemini CLI"); empty means all.
func (d *DB) QueryPeriodStatsSince(since time.Time, deviceID string, source string) (float64, int, int, int, int, error) {
	var cost sql.NullFloat64
	var inTok, cacheTok, cacheCreationTok, outTok sql.NullInt64
	query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_records WHERE " + activeUsagePredicate + " AND julianday(log_timestamp) >= julianday(?)"
	args := []interface{}{formatLogTimestamp(since)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	err := d.conn.QueryRow(query, args...).Scan(&cost, &inTok, &cacheTok, &cacheCreationTok, &outTok)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return cost.Float64, int(inTok.Int64), int(cacheTok.Int64), int(cacheCreationTok.Int64), int(outTok.Int64), nil
}

// QueryPeriodStatsBuckets returns multiple period aggregates in a single scan.
// A zero Since value means the all-time bucket.
// QueryPeriodStatsBuckets returns multiple period aggregates in a single scan.
// A zero Since value means the all-time bucket.
func (d *DB) QueryPeriodStatsBuckets(windows []PeriodStatsWindow, deviceID string, source string) ([]PeriodStatsBucket, error) {
	if len(windows) == 0 {
		return []PeriodStatsBucket{}, nil
	}

	var query strings.Builder
	query.WriteString("SELECT ")
	args := make([]interface{}, 0, len(windows)*5+2)
	for i, window := range windows {
		if i > 0 {
			query.WriteString(", ")
		}
		conditional := !window.Since.IsZero()
		appendPeriodAggregateExpr(&query, "cost_usd", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "input_tokens", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "cached_tokens", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "cache_creation_tokens", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "output_tokens", conditional)
		if conditional {
			since := formatLogTimestamp(window.Since)
			args = append(args, since, since, since, since, since)
		}
	}
	query.WriteString(" FROM usage_records WHERE ")
	query.WriteString(activeUsagePredicate)

	if deviceID != "" && deviceID != "all" {
		query.WriteString(" AND device_id = ?")
		args = append(args, deviceID)
	}
	if source != "" {
		query.WriteString(" AND source = ?")
		args = append(args, source)
	}

	values := make([]struct {
		cost          sql.NullFloat64
		input         sql.NullInt64
		cached        sql.NullInt64
		cacheCreation sql.NullInt64
		output        sql.NullInt64
	}, len(windows))
	scanArgs := make([]interface{}, 0, len(windows)*5)
	for i := range values {
		scanArgs = append(scanArgs, &values[i].cost, &values[i].input, &values[i].cached, &values[i].cacheCreation, &values[i].output)
	}

	if err := d.conn.QueryRow(query.String(), args...).Scan(scanArgs...); err != nil {
		return nil, err
	}

	buckets := make([]PeriodStatsBucket, 0, len(windows))
	for i, window := range windows {
		buckets = append(buckets, PeriodStatsBucket{
			Label:               window.Label,
			Cost:                values[i].cost.Float64,
			InputTokens:         int(values[i].input.Int64),
			CachedTokens:        int(values[i].cached.Int64),
			CacheCreationTokens: int(values[i].cacheCreation.Int64),
			OutputTokens:        int(values[i].output.Int64),
		})
	}
	return buckets, nil
}
func appendPeriodAggregateExpr(query *strings.Builder, column string, conditional bool) {
	if conditional {
		query.WriteString("COALESCE(SUM(CASE WHEN julianday(log_timestamp) >= julianday(?) THEN ")
		query.WriteString(column)
		query.WriteString(" ELSE 0 END), 0)")
		return
	}
	query.WriteString("COALESCE(SUM(")
	query.WriteString(column)
	query.WriteString("), 0)")
}

// QueryPeriodStatsAll returns total cumulative cost and token breakdown.
// source filters by source column; empty means all.
// QueryPeriodStatsAll returns total cumulative cost and token breakdown.
// source filters by source column; empty means all.
func (d *DB) QueryPeriodStatsAll(deviceID string, source string) (float64, int, int, int, int, error) {
	var cost sql.NullFloat64
	var inTok, cacheTok, cacheCreationTok, outTok sql.NullInt64
	query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_records WHERE " + activeUsagePredicate
	var args []interface{}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	err := d.conn.QueryRow(query, args...).Scan(&cost, &inTok, &cacheTok, &cacheCreationTok, &outTok)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return cost.Float64, int(inTok.Int64), int(cacheTok.Int64), int(cacheCreationTok.Int64), int(outTok.Int64), nil
}

// QueryTokenSourceSummaries returns per-source token totals in one aggregate query.
// QueryTokenSourceSummaries returns per-source token totals in one aggregate query.
func (d *DB) QueryTokenSourceSummaries(since time.Time, deviceID string) ([]model.TokenSourceSummary, error) {
	query := `SELECT source,
		COALESCE(SUM(CASE WHEN julianday(log_timestamp) >= julianday(?) THEN input_tokens + output_tokens ELSE 0 END), 0),
		COALESCE(SUM(input_tokens + output_tokens), 0),
		COALESCE(SUM(cost_usd), 0)
		FROM usage_records
		WHERE ` + activeUsagePredicate
	args := []interface{}{formatLogTimestamp(since)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}

	query += " GROUP BY source ORDER BY source"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []model.TokenSourceSummary
	for rows.Next() {
		var summary model.TokenSourceSummary
		if err := rows.Scan(&summary.Source, &summary.Tokens24h, &summary.TokensTotal, &summary.CostTotal); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// QueryDailyCostBuckets returns non-empty daily cost buckets in the given date window.
func (d *DB) QueryDailyCostBuckets(startInclusive time.Time, endExclusive time.Time, deviceID string, source string) ([]model.DailyCostBucket, error) {
	query := `SELECT date(log_timestamp), COALESCE(SUM(cost_usd), 0)
		FROM usage_records
		WHERE ` + activeUsagePredicate + ` AND julianday(log_timestamp) >= julianday(?) AND julianday(log_timestamp) < julianday(?)`
	args := []interface{}{formatLogTimestamp(startInclusive), formatLogTimestamp(endExclusive)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += " GROUP BY date(log_timestamp) ORDER BY date(log_timestamp)"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := []model.DailyCostBucket{}
	for rows.Next() {
		var bucket model.DailyCostBucket
		if err := rows.Scan(&bucket.Date, &bucket.Cost); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

// QueryCalendarHeatmapBuckets returns non-empty daily token buckets for a calendar heatmap.
func (d *DB) QueryCalendarHeatmapBuckets(startInclusive time.Time, endExclusive time.Time, deviceID string, source string) ([]model.CalendarHeatmapBucket, error) {
	query := `SELECT date(log_timestamp), COALESCE(SUM(input_tokens + output_tokens), 0), COALESCE(SUM(cost_usd), 0), COUNT(*)
		FROM usage_records
		WHERE ` + activeUsagePredicate + ` AND julianday(log_timestamp) >= julianday(?) AND julianday(log_timestamp) < julianday(?)`
	args := []interface{}{formatLogTimestamp(startInclusive), formatLogTimestamp(endExclusive)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += " GROUP BY date(log_timestamp) ORDER BY date(log_timestamp)"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := []model.CalendarHeatmapBucket{}
	for rows.Next() {
		var bucket model.CalendarHeatmapBucket
		if err := rows.Scan(&bucket.Date, &bucket.Tokens, &bucket.Cost, &bucket.Events); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

// QueryToolShareBuckets returns token totals by source/tool in the given date window.
func (d *DB) QueryToolShareBuckets(startInclusive time.Time, endExclusive time.Time, deviceID string, source string) ([]model.ToolShareBucket, error) {
	query := `SELECT source, COALESCE(SUM(input_tokens + output_tokens), 0), COALESCE(SUM(cost_usd), 0), COUNT(*)
		FROM usage_records
		WHERE ` + activeUsagePredicate + ` AND julianday(log_timestamp) >= julianday(?) AND julianday(log_timestamp) < julianday(?)`
	args := []interface{}{formatLogTimestamp(startInclusive), formatLogTimestamp(endExclusive)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += " GROUP BY source ORDER BY COALESCE(SUM(input_tokens + output_tokens), 0) DESC, source ASC"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := []model.ToolShareBucket{}
	for rows.Next() {
		var bucket model.ToolShareBucket
		if err := rows.Scan(&bucket.Source, &bucket.Tokens, &bucket.TotalCost, &bucket.Events); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
func (d *DB) QueryStatsSince(since time.Time, deviceID string, source string) ([]ModelStat, error) {
	query := `
		SELECT source, model, COUNT(*) as events,
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
		GROUP BY source, model
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ModelStat
	for rows.Next() {
		var s ModelStat
		if err := rows.Scan(&s.Source, &s.Model, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QuerySourceTotalsSince returns per-source aggregate stats without model/project details.
// QueryProjectStatsSince returns per-project stats since the given time.
func (d *DB) QueryProjectStatsSince(since time.Time, deviceID string, source string) ([]model.ProjectStat, error) {
	query := `
		SELECT COALESCE(project, 'Default') as project, COUNT(*) as events,
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
		GROUP BY project
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []model.ProjectStat
	for rows.Next() {
		var s model.ProjectStat
		if err := rows.Scan(&s.Project, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		s.CacheHitRate = model.CacheHitRatePercent(s.InputTokens, s.CachedTokens)
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QueryDevices returns a list of unique devices.
// QueryUsageRecords returns individual usage records since the given time.
func (d *DB) QueryUsageRecords(since time.Time, deviceID string) ([]UsageRecord, error) {
	query := `SELECT model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd
		FROM usage_records WHERE COALESCE(superseded, 0) = 0`
	var args []interface{}

	if !since.IsZero() {
		query += " AND julianday(log_timestamp) >= julianday(?)"
		args = append(args, formatLogTimestamp(since))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(&r.Model, &r.InputTokens, &r.CachedTokens, &r.CacheCreationTokens, &r.OutputTokens, &r.CostUSD); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// QuerySyncRecordsSince returns raw records for P2P LAN DB synchronization.
// The cursor is based on updated_at, not log_timestamp, so historical repairs
// and superseded markers propagate even when the underlying usage happened long ago.
