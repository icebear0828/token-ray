package db

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

var csvHeader = []string{
	"log_timestamp", "source", "model",
	"input_tokens", "cached_tokens", "cache_creation_tokens", "output_tokens",
	"cost_usd", "file_path", "device_id",
	"duration_ms", "timed_output_tokens",
}

// Older exports predate the timing columns, so only the original columns are
// required on import.
var requiredCSVHeader = csvHeader[:10]

// ExportCSV writes all usage records as CSV to w.
// If deviceID is non-empty and not "all", only that device's records are exported.
// Returns the number of rows written.
func (d *DB) ExportCSV(w io.Writer, deviceID string) (int, error) {
	query := `SELECT log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, duration_ms, timed_output_tokens
		FROM usage_records WHERE ` + activeUsagePredicate
	var args []interface{}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	query += " ORDER BY julianday(log_timestamp), id"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return 0, err
	}

	count := 0
	for rows.Next() {
		var logTS, source, mdl, filePath, devID string
		var inTok, cacheTok, cacheCreationTok, outTok int
		var durationMS int64
		var timedOutputTokens int
		var cost float64
		if err := rows.Scan(&logTS, &source, &mdl, &inTok, &cacheTok, &cacheCreationTok, &outTok, &cost, &filePath, &devID, &durationMS, &timedOutputTokens); err != nil {
			return count, err
		}
		record := []string{
			logTS, source, mdl,
			strconv.Itoa(inTok), strconv.Itoa(cacheTok), strconv.Itoa(cacheCreationTok), strconv.Itoa(outTok),
			fmt.Sprintf("%.6f", cost), filePath, devID,
			strconv.FormatInt(durationMS, 10), strconv.Itoa(timedOutputTokens),
		}
		if err := cw.Write(record); err != nil {
			return count, err
		}
		count++
	}
	cw.Flush()
	return count, cw.Error()
}

// ImportCSV reads CSV records from r and inserts them into the database.
// Duplicates are silently skipped via INSERT OR IGNORE.
// Returns (imported, skipped, error).
func (d *DB) ImportCSV(r io.Reader) (int, int, error) {
	cr := csv.NewReader(r)

	// Read and validate header
	header, err := cr.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("read header: %w", err)
	}
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[col] = i
	}
	for _, required := range requiredCSVHeader {
		if _, ok := colIdx[required]; !ok {
			return 0, 0, fmt.Errorf("missing required column: %s", required)
		}
	}

	imported, skipped := 0, 0
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return imported, skipped, fmt.Errorf("read row %d: %w", imported+skipped+1, err)
		}

		logTS := record[colIdx["log_timestamp"]]
		source := record[colIdx["source"]]
		mdl := record[colIdx["model"]]
		inTok, _ := strconv.Atoi(record[colIdx["input_tokens"]])
		cacheTok, _ := strconv.Atoi(record[colIdx["cached_tokens"]])
		cacheCreationTok, _ := strconv.Atoi(record[colIdx["cache_creation_tokens"]])
		outTok, _ := strconv.Atoi(record[colIdx["output_tokens"]])
		cost, _ := strconv.ParseFloat(record[colIdx["cost_usd"]], 64)
		filePath := record[colIdx["file_path"]]
		devID := record[colIdx["device_id"]]
		var durationMS int64
		if idx, ok := colIdx["duration_ms"]; ok && idx < len(record) {
			durationMS, _ = strconv.ParseInt(record[idx], 10, 64)
		}
		var timedOutputTokens int
		if idx, ok := colIdx["timed_output_tokens"]; ok && idx < len(record) {
			timedOutputTokens, _ = strconv.Atoi(record[idx])
		}

		// Parse timestamp for canonical formatting.
		tsStr := logTS
		ts, terr := parseLogTimestamp(logTS)
		if terr == nil {
			tsStr = formatLogTimestamp(ts)
		}

		query := `INSERT OR IGNORE INTO usage_records
			(log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, duration_ms, timed_output_tokens, cost_usd, file_path, device_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		result, err := d.conn.Exec(query, tsStr, source, mdl, inTok, cacheTok, cacheCreationTok, outTok, durationMS, timedOutputTokens, cost, filePath, devID)
		if err != nil {
			return imported, skipped, fmt.Errorf("insert row %d: %w", imported+skipped+1, err)
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			imported++
		} else {
			skipped++
		}
	}

	return imported, skipped, nil
}
