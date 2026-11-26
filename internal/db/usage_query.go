package db

import (
	"ai-flight-dashboard/internal/model"
	"database/sql"
	"slices"
	"strconv"
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
func (d *DB) QueryPeriodStatsBuckets(windows []PeriodStatsWindow, deviceID string, source string) ([]PeriodStatsBucket, error) {
	if len(windows) == 0 {
		return []PeriodStatsBucket{}, nil
	}

	// Collect unique conditional window timestamps sorted newest-first so each
	// active row falls into a single disjoint bucket in one pass, then accumulate
	// prefix sums in Go.
	type condBound struct {
		since time.Time
		ts    string
	}
	uniqueBounds := make([]condBound, 0, len(windows))
	for _, window := range windows {
		if window.Since.IsZero() {
			continue
		}
		ts := formatLogTimestamp(window.Since)
		seen := false
		for _, b := range uniqueBounds {
			if b.ts == ts {
				seen = true
				break
			}
		}
		if !seen {
			uniqueBounds = append(uniqueBounds, condBound{since: window.Since.UTC(), ts: ts})
		}
	}
	slices.SortFunc(uniqueBounds, func(a, b condBound) int {
		// Newest (largest Since) first.
		return b.since.Compare(a.since)
	})

	windowBoundIdx := make([]int, len(windows))
	for i, window := range windows {
		if window.Since.IsZero() {
			windowBoundIdx[i] = len(uniqueBounds)
			continue
		}
		ts := formatLogTimestamp(window.Since)
		for j, b := range uniqueBounds {
			if b.ts == ts {
				windowBoundIdx[i] = j
				break
			}
		}
	}

	numGroups := len(uniqueBounds) + 1
	type groupAgg struct {
		cost          float64
		input         int
		cached        int
		cacheCreation int
		output        int
		timedOutput   int
		duration      int64
		timedEvents   int
	}
	groups := make([]groupAgg, numGroups)

	if len(uniqueBounds) == 0 {
		query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(timed_output_tokens), 0), COALESCE(SUM(duration_ms), 0), COALESCE(SUM(CASE WHEN duration_ms > 0 THEN 1 ELSE 0 END), 0) FROM usage_records WHERE " + activeUsagePredicate
		var args []interface{}
		if deviceID != "" && deviceID != "all" {
			query += " AND device_id = ?"
			args = append(args, deviceID)
		}
		if source != "" {
			query += " AND source = ?"
			args = append(args, source)
		}
		var g groupAgg
		if err := d.conn.QueryRow(query, args...).Scan(&g.cost, &g.input, &g.cached, &g.cacheCreation, &g.output, &g.timedOutput, &g.duration, &g.timedEvents); err != nil {
			return nil, err
		}
		groups[0] = g
	} else {
		var query strings.Builder
		args := make([]interface{}, 0, len(uniqueBounds)+2)
		query.WriteString("WITH bounds AS MATERIALIZED (SELECT ")
		for i, b := range uniqueBounds {
			if i > 0 {
				query.WriteString(", ")
			}
			query.WriteString("julianday(?) AS b")
			query.WriteString(strconv.Itoa(i))
			args = append(args, b.ts)
		}
		query.WriteString("), active AS MATERIALIZED (SELECT julianday(log_timestamp) AS jd, cost_usd, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, timed_output_tokens, duration_ms FROM usage_records WHERE ")
		query.WriteString(activeUsagePredicate)
		if deviceID != "" && deviceID != "all" {
			query.WriteString(" AND device_id = ?")
			args = append(args, deviceID)
		}
		if source != "" {
			query.WriteString(" AND source = ?")
			args = append(args, source)
		}
		query.WriteString(") SELECT CASE")
		for i := range uniqueBounds {
			query.WriteString(" WHEN active.jd >= (SELECT b")
			query.WriteString(strconv.Itoa(i))
			query.WriteString(" FROM bounds) THEN ")
			query.WriteString(strconv.Itoa(i))
		}
		query.WriteString(" ELSE ")
		query.WriteString(strconv.Itoa(len(uniqueBounds)))
		query.WriteString(" END AS grp, COALESCE(SUM(active.cost_usd), 0), COALESCE(SUM(active.input_tokens), 0), COALESCE(SUM(active.cached_tokens), 0), COALESCE(SUM(active.cache_creation_tokens), 0), COALESCE(SUM(active.output_tokens), 0), COALESCE(SUM(active.timed_output_tokens), 0), COALESCE(SUM(active.duration_ms), 0), COALESCE(SUM(CASE WHEN active.duration_ms > 0 THEN 1 ELSE 0 END), 0) FROM active GROUP BY grp")

		rows, err := d.conn.Query(query.String(), args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var grp int
			var g groupAgg
			if err := rows.Scan(&grp, &g.cost, &g.input, &g.cached, &g.cacheCreation, &g.output, &g.timedOutput, &g.duration, &g.timedEvents); err != nil {
				return nil, err
			}
			if grp >= 0 && grp < numGroups {
				groups[grp] = g
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Prefix sum: groups[i] accumulates all rows >= uniqueBounds[i], and
	// groups[len(uniqueBounds)] holds the all-time totals.
	for i := 1; i < len(groups); i++ {
		groups[i].cost += groups[i-1].cost
		groups[i].input += groups[i-1].input
		groups[i].cached += groups[i-1].cached
		groups[i].cacheCreation += groups[i-1].cacheCreation
		groups[i].output += groups[i-1].output
		groups[i].timedOutput += groups[i-1].timedOutput
		groups[i].duration += groups[i-1].duration
		groups[i].timedEvents += groups[i-1].timedEvents
	}

	buckets := make([]PeriodStatsBucket, 0, len(windows))
	for i, window := range windows {
		g := groups[windowBoundIdx[i]]
		buckets = append(buckets, PeriodStatsBucket{
			Label:               window.Label,
			Cost:                g.cost,
			InputTokens:         g.input,
			CachedTokens:        g.cached,
			CacheCreationTokens: g.cacheCreation,
			OutputTokens:        g.output,
			TimedOutputTokens:   g.timedOutput,
			DurationMS:          g.duration,
			TimedEvents:         g.timedEvents,
		})
	}
	return buckets, nil
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

	summaries := make([]model.TokenSourceSummary, 0, 8)
	for rows.Next() {
		var summary model.TokenSourceSummary
		if err := rows.Scan(&summary.Source, &summary.Tokens24h, &summary.TokensTotal, &summary.CostTotal); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// QueryDailyCostBuckets returns non-empty cost/token buckets in the given
// window, grouped at the requested granularity. A zero start means no lower
// bound (used by the all-time chart).
func (d *DB) QueryDailyCostBuckets(startInclusive time.Time, endExclusive time.Time, granularity string, deviceID string, source string) ([]model.DailyCostBucket, error) {
	bucketExpr := bucketExpression(granularity)
	query := `SELECT ` + bucketExpr + `, COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens + output_tokens), 0)
		FROM usage_records
		WHERE ` + activeUsagePredicate + ` AND julianday(log_timestamp) < julianday(?)`
	args := []interface{}{formatLogTimestamp(endExclusive)}

	if !startInclusive.IsZero() {
		query += ` AND julianday(log_timestamp) >= julianday(?)`
		args = append(args, formatLogTimestamp(startInclusive))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += " GROUP BY " + bucketExpr + " ORDER BY " + bucketExpr

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := make([]model.DailyCostBucket, 0, 32)
	for rows.Next() {
		var bucket model.DailyCostBucket
		if err := rows.Scan(&bucket.Date, &bucket.Cost, &bucket.Tokens); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

func bucketExpression(granularity string) string {
	switch granularity {
	case BucketHourly:
		return "strftime('%Y-%m-%d %H:00', log_timestamp)"
	case BucketMonthly:
		return "strftime('%Y-%m', log_timestamp)"
	default:
		return "date(log_timestamp)"
	}
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

	buckets := make([]model.CalendarHeatmapBucket, 0, 32)
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

	buckets := make([]model.ToolShareBucket, 0, 8)
	for rows.Next() {
		var bucket model.ToolShareBucket
		if err := rows.Scan(&bucket.Source, &bucket.Tokens, &bucket.TotalCost, &bucket.Events); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

// QueryUsageByFileUUIDs returns the usage rows already stored for a file,
// keyed by UUID. Sources that re-read a transcript use it to skip unchanged
// historical records instead of re-emitting them.
func (d *DB) QueryUsageByFileUUIDs(filePath string, deviceID string) (map[string]model.TokenUsage, error) {
	query := `SELECT uuid, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens
		FROM usage_records
		WHERE file_path = ? AND device_id = ? AND uuid IS NOT NULL AND uuid != '' AND COALESCE(superseded, 0) = 0`
	rows, err := d.conn.Query(query, filePath, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]model.TokenUsage)
	for rows.Next() {
		var u model.TokenUsage
		if err := rows.Scan(&u.UUID, &u.Source, &u.Model, &u.Project, &u.InputTokens, &u.CachedTokens, &u.CacheCreationTokens, &u.OutputTokens); err != nil {
			return nil, err
		}
		out[u.UUID] = u
	}
	return out, rows.Err()
}

// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
func (d *DB) QueryStatsSince(since time.Time, deviceID string, source string) ([]ModelStat, error) {
	query := `
		SELECT source, model, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(timed_output_tokens), SUM(duration_ms),
			SUM(CASE WHEN duration_ms > 0 THEN 1 ELSE 0 END),
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

	stats := make([]ModelStat, 0, 16)
	for rows.Next() {
		var s ModelStat
		if err := rows.Scan(&s.Source, &s.Model, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens,
			&s.TimedOutputTokens, &s.DurationMS, &s.TimedEvents, &s.TotalCost); err != nil {
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
			SUM(timed_output_tokens), SUM(duration_ms),
			SUM(CASE WHEN duration_ms > 0 THEN 1 ELSE 0 END),
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

	stats := make([]model.ProjectStat, 0, 16)
	for rows.Next() {
		var s model.ProjectStat
		if err := rows.Scan(&s.Project, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens,
			&s.TimedOutputTokens, &s.DurationMS, &s.TimedEvents, &s.TotalCost); err != nil {
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

	records := make([]UsageRecord, 0, 64)
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
