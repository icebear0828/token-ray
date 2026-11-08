package db

import (
	"database/sql"
)

// GetOffset returns the last scanned byte offset for a file. Returns 0 if not found.
func (d *DB) GetOffset(filePath string) (int64, error) {
	var offset int64
	err := d.conn.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", filePath).Scan(&offset)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return offset, err
}

// AllOffsets returns every recorded scan offset keyed by file path. Scanners
// use it to avoid issuing one query per known file.
func (d *DB) AllOffsets() (map[string]int64, error) {
	rows, err := d.conn.Query("SELECT file_path, byte_offset FROM scan_offsets WHERE file_path NOT LIKE 'migration:%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	offsets := make(map[string]int64)
	for rows.Next() {
		var path string
		var offset int64
		if err := rows.Scan(&path, &offset); err != nil {
			return nil, err
		}
		offsets[path] = offset
	}
	return offsets, rows.Err()
}

// SetOffset upserts the byte offset for a file.
// SetOffset upserts the byte offset for a file.
func (d *DB) SetOffset(filePath string, offset int64) error {
	query := `INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, ?)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`
	_, err := d.conn.Exec(query, filePath, offset)
	return err
}

// ResetOffsetsLike resets scan offsets matching a SQL LIKE pattern so files are
// re-read by the incremental scanner. UUID/upsert dedup keeps the rescan safe.
// ResetOffsetsLike resets scan offsets matching a SQL LIKE pattern so files are
// re-read by the incremental scanner. UUID/upsert dedup keeps the rescan safe.
func (d *DB) ResetOffsetsLike(pattern string) (int64, error) {
	result, err := d.conn.Exec("UPDATE scan_offsets SET byte_offset = 0 WHERE file_path LIKE ?", pattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ResetOffset resets the scan offset for one exact file path.
// ResetOffset resets the scan offset for one exact file path.
func (d *DB) ResetOffset(filePath string) (int64, error) {
	result, err := d.conn.Exec("UPDATE scan_offsets SET byte_offset = 0 WHERE file_path = ?", filePath)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SupersedeLegacyUsageBySourceFilePathsAndDevices hides legacy no-UUID rows
// matching exact source, file path, and device IDs. Empty path/device lists are
// no-ops. Rows are retained for audit and recovery.
// UpsertKnownDir records a directory known to contain JSONL files.
func (d *DB) UpsertKnownDir(dirPath string) error {
	query := `INSERT INTO known_dirs (dir_path) VALUES (?)
	ON CONFLICT(dir_path) DO UPDATE SET last_seen = CURRENT_TIMESTAMP`
	_, err := d.conn.Exec(query, dirPath)
	return err
}

// ListKnownDirs returns all cached JSONL directories.
// ListKnownDirs returns all cached JSONL directories.
func (d *DB) ListKnownDirs() ([]string, error) {
	rows, err := d.conn.Query("SELECT dir_path FROM known_dirs ORDER BY dir_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dirs []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		dirs = append(dirs, p)
	}
	return dirs, rows.Err()
}

// ListKnownFiles returns all file paths cached in scan_offsets.
// ListKnownFiles returns all file paths cached in scan_offsets.
func (d *DB) ListKnownFiles() ([]string, error) {
	rows, err := d.conn.Query("SELECT file_path FROM scan_offsets WHERE file_path NOT LIKE 'migration:%' ORDER BY file_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		files = append(files, p)
	}
	return files, rows.Err()
}

// BackfillProjectsFromFilePaths recalculates Default project names for records
// that still have their originating log path.
// BackfillProjectsFromFilePaths recalculates Default project names for records
// that still have their originating log path.
func (d *DB) BackfillProjectsFromFilePaths(extractProject func(string) string) (int64, error) {
	rows, err := d.conn.Query("SELECT id, file_path FROM usage_records WHERE " + activeUsagePredicate + " AND file_path != '' AND (project IS NULL OR project = '' OR project = 'Default')")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type update struct {
		id      int64
		project string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var filePath string
		if err := rows.Scan(&id, &filePath); err != nil {
			return 0, err
		}
		project := extractProject(filePath)
		if project != "" && project != "Default" {
			updates = append(updates, update{id: id, project: project})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(updates) == 0 {
		return 0, nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare("UPDATE usage_records SET project = ? WHERE id = ?")
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	var changed int64
	for _, u := range updates {
		result, err := stmt.Exec(u.project, u.id)
		if err != nil {
			tx.Rollback()
			return changed, err
		}
		n, _ := result.RowsAffected()
		changed += n
	}
	if err := tx.Commit(); err != nil {
		return changed, err
	}
	return changed, nil
}

// DeduplicateExisting removes duplicate usage records, keeping the one with the lowest rowid.
// Returns the number of rows removed.
