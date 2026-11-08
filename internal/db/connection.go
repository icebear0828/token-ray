package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(dsn string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Enforce single connection pool to prevent concurrent write locks in WAL mode.
	// Keeping one idle connection also keeps the per-connection PRAGMAs below in
	// effect for the lifetime of the process.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	// WAL mode for concurrent read/write. NORMAL is safe under WAL (a power loss
	// can only drop the last transaction, never corrupt the database) and avoids
	// an fsync on every commit. The remaining pragmas keep temp tables in memory
	// and give SQLite a 64MB page cache to cut disk reads.
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA synchronous=NORMAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.Exec("PRAGMA temp_store=MEMORY")
	conn.Exec("PRAGMA cache_size=-64000")

	if err := initSchema(conn); err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}
func (d *DB) Close() error {
	return d.conn.Close()
}

// UpsertKnownDir records a directory known to contain JSONL files.
