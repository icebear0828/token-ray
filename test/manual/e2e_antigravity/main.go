package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"ai-flight-dashboard/internal/agyscanner"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	tmpDir, err := os.MkdirTemp("", "agy-e2e-*")
	if err != nil {
		log.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "usage.db")
	origDBPath := filepath.Join(os.Getenv("HOME"), ".ai-flight-dashboard", "stats", "usage.db")

	// Copy existing db if available to test backfill on real historical DB
	if origF, err := os.Open(origDBPath); err == nil {
		defer origF.Close()
		if targetF, err := os.Create(dbPath); err == nil {
			defer targetF.Close()
			_, _ = io.Copy(targetF, origF)
		}
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	calc, err := calculator.New("cmd/dashboard/pricing_table.json")
	if err != nil {
		log.Fatalf("create calc: %v", err)
	}
	scanner := agyscanner.New(database, calc, "e2e-device")

	rawDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("open raw sqlite: %v", err)
	}
	defer rawDB.Close()

	fmt.Println("=== Starting E2E Verification for Antigravity Scanner ===")

	for run := 1; run <= 3; run++ {
		start := time.Now()
		n, err := scanner.Scan(nil)
		duration := time.Since(start)
		if err != nil {
			log.Fatalf("Run %d failed: %v", run, err)
		}

		var count int
		var latestTime, latestModel, latestProject string
		var totalIn, totalOut int64
		_ = rawDB.QueryRow(`
			SELECT count(*), coalesce(max(timestamp), ''), coalesce(sum(input_tokens), 0), coalesce(sum(output_tokens), 0)
			FROM usage_records WHERE source = 'Antigravity'
		`).Scan(&count, &latestTime, &totalIn, &totalOut)

		_ = rawDB.QueryRow(`
			SELECT coalesce(model, ''), coalesce(project, '')
			FROM usage_records WHERE source = 'Antigravity' ORDER BY timestamp DESC LIMIT 1
		`).Scan(&latestModel, &latestProject)

		fmt.Printf("Run %d (took %v): inserted=%d records | total Antigravity in DB=%d | latest_time=%s model=%s project=%s in=%d out=%d\n",
			run, duration, n, count, latestTime, latestModel, latestProject, totalIn, totalOut)

		if run > 1 && n != 0 {
			log.Fatalf("Idempotency violation: Run %d inserted %d records (expected 0)", run, n)
		}
	}

	fmt.Println("=== E2E 3 consecutive runs succeeded! ===")
}
