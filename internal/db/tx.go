package db

import (
	"database/sql"
	"time"

	"ai-flight-dashboard/internal/model"
)

// Tx is a write transaction. It exposes the persistence surface sources need
// so a batch of usage rows, offsets, and directory markers can be committed
// with a single fsync.
type Tx struct {
	tx *sql.Tx
}

// RunInTx runs fn inside a transaction and commits it when fn returns nil.
func (d *DB) RunInTx(fn func(*Tx) error) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	if err := fn(&Tx{tx: tx}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (t *Tx) InsertUsageWithTime(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string) error {
	return insertUsageWithTimeExec(t.tx, u, cost, logTS, filePath, deviceID, false)
}

func (t *Tx) GetOffset(filePath string) (int64, error) {
	var offset int64
	err := t.tx.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", filePath).Scan(&offset)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return offset, err
}

func (t *Tx) SetOffset(filePath string, offset int64) error {
	query := `INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, ?)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`
	_, err := t.tx.Exec(query, filePath, offset)
	return err
}

func (t *Tx) AllOffsets() (map[string]int64, error) {
	rows, err := t.tx.Query("SELECT file_path, byte_offset FROM scan_offsets WHERE file_path NOT LIKE 'migration:%'")
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

func (t *Tx) ListKnownFiles() ([]string, error) {
	rows, err := t.tx.Query("SELECT file_path FROM scan_offsets WHERE file_path NOT LIKE 'migration:%' ORDER BY file_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		files = append(files, path)
	}
	return files, rows.Err()
}

func (t *Tx) UpsertKnownDir(dirPath string) error {
	query := `INSERT INTO known_dirs (dir_path) VALUES (?)
	ON CONFLICT(dir_path) DO UPDATE SET last_seen = CURRENT_TIMESTAMP`
	_, err := t.tx.Exec(query, dirPath)
	return err
}

func (t *Tx) SupersedeUsageBySourceFilePathDeviceUUIDPrefix(source string, filePath string, deviceID string, uuidPrefix string) (int64, error) {
	if source == "" || filePath == "" || deviceID == "" || uuidPrefix == "" {
		return 0, nil
	}
	result, err := t.tx.Exec(
		"UPDATE usage_records SET superseded = 1, updated_at = ? WHERE source = ? AND file_path = ? AND device_id = ? AND uuid LIKE ? AND COALESCE(superseded, 0) = 0",
		formatLogTimestamp(time.Now().UTC()), source, filePath, deviceID, uuidPrefix+"%",
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
