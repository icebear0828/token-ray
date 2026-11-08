package main

import (
	"ai-flight-dashboard/internal/applock"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/db"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func openDB() (*db.DB, *applock.Lock) {
	appDataDir := config.GetDataDir()
	lock := acquireProcessLock(appDataDir, false)
	dbPath := filepath.Join(appDataDir, "stats", "usage.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	database, err := db.New(dbPath)
	if err != nil {
		lock.Release()
		log.Fatalf("Failed to open database: %v", err)
	}
	return database, lock
}
func acquireProcessLock(dataDir string, gui bool) *applock.Lock {
	lockPath := filepath.Join(dataDir, "dashboard.lock")
	lock, err := applock.TryAcquire(lockPath)
	if err == nil {
		return lock
	}
	if errors.Is(err, applock.ErrAlreadyLocked) {
		if gui {
			showAlreadyRunningMessage(dataDir)
			os.Exit(0)
		}
		log.Fatalf("Another AI Flight Dashboard process is already using %s. Stop it before starting a second dashboard or running a repair/import/dedup command.", dataDir)
	}
	log.Fatalf("Failed to acquire process lock %s: %v", lockPath, err)
	return nil
}
func runExport(deviceID string) {
	database, lock := openDB()
	defer lock.Release()
	defer database.Close()

	filter := ""
	if deviceID != "local" && deviceID != "" {
		filter = deviceID
	}

	count, err := database.ExportCSV(os.Stdout, filter)
	if err != nil {
		log.Fatalf("Export failed: %v", err)
	}
	fmt.Fprintf(os.Stderr, "✅ Exported %d records", count)
	if filter != "" {
		fmt.Fprintf(os.Stderr, " (device=%s)", filter)
	}
	fmt.Fprintln(os.Stderr)
}
func runImport(filePath string) {
	database, lock := openDB()
	defer lock.Release()
	defer database.Close()

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", filePath, err)
	}
	defer file.Close()

	imported, skipped, err := database.ImportCSV(file)
	if err != nil {
		log.Fatalf("Import failed: %v", err)
	}
	fmt.Printf("✅ Import complete: %d imported, %d skipped (duplicates)\n", imported, skipped)
}
func runDedup() {
	database, lock := openDB()
	defer lock.Release()
	defer database.Close()

	removed, err := database.DeduplicateExisting()
	if err != nil {
		log.Fatalf("Dedup failed: %v", err)
	}
	fmt.Printf("✅ Removed %d duplicate records\n", removed)
}
