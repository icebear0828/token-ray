package main

import (
	"ai-flight-dashboard/internal/agyscanner"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/scanner"
	"ai-flight-dashboard/internal/watcher"
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runRepairHistory(database *db.DB, calc *calculator.Calculator, deviceID string, scanDirs []string) {
	geminiFiles, err := discoverGeminiHistoryFiles(scanDirs)
	if err != nil {
		log.Fatalf("Failed to discover Gemini history files: %v", err)
	}

	var resetFiles int64
	for _, filePath := range geminiFiles {
		n, err := database.ResetOffset(filePath)
		if err != nil {
			log.Fatalf("Failed to reset Gemini offset for %q: %v", filePath, err)
		}
		resetFiles += n
	}
	for _, pattern := range []string{
		"%/.claude/projects/%",
		"%\\.claude\\projects\\%",
	} {
		n, err := database.ResetOffsetsLike(pattern)
		if err != nil {
			log.Fatalf("Failed to reset Claude offsets for %q: %v", pattern, err)
		}
		resetFiles += n
	}
	if err := database.SetOffset(codexusage.OffsetKey, 0); err != nil {
		log.Fatalf("Failed to reset Codex offset: %v", err)
	}

	s := scanner.New(database, calc, deviceID)
	scanned, err := s.ScanAllStrict(scanDirs, nil)
	if err != nil {
		log.Fatalf("History repair scan failed: %v", err)
	}
	codexScanner := codexusage.New(database, calc, deviceID)
	codexScanned, err := codexScanner.Scan(nil)
	if err != nil {
		log.Fatalf("Codex repair scan failed: %v", err)
	}

	agyScanner := agyscanner.New(database, calc, deviceID)
	agyScanned, err := agyScanner.Backfill(nil)
	if err != nil {
		log.Printf("Antigravity backfill warning: %v", err)
	}

	supersededGemini, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", geminiFiles, localRepairDeviceIDs(deviceID))
	if err != nil {
		log.Fatalf("Failed to supersede replayed Gemini rows: %v", err)
	}

	fmt.Printf("✅ History repair complete: superseded %d old Gemini rows, reset %d file offsets, replayed %d JSONL records, %d Codex events, %d Antigravity events\n", supersededGemini, resetFiles, scanned, codexScanned, agyScanned)
}
func discoverGeminiHistoryFiles(scanDirs []string) ([]string, error) {
	return discoverHistoryFiles(scanDirs, func(path string) bool {
		if !isGeminiHistoryFile(path) {
			return false
		}
		ok, err := fileHasUsageSource(path, "Gemini CLI")
		return err == nil && ok
	})
}
func discoverHistoryFiles(scanDirs []string, match func(string) bool) ([]string, error) {
	seen := make(map[string]struct{})
	for _, dir := range scanDirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !match(path) {
				return nil
			}
			seen[path] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}
func isGeminiHistoryFile(path string) bool {
	if !strings.HasSuffix(path, ".jsonl") {
		return false
	}
	return strings.Contains(filepath.ToSlash(path), "/.gemini/tmp/")
}
func fileHasUsageSource(path string, source string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if u, ok := watcher.ParseLine(line); ok && u.Source == source {
				return true, nil
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
func localRepairDeviceIDs(deviceID string) []string {
	ids := []string{deviceID, "local", ""}
	seen := make(map[string]struct{}, len(ids))
	unique := ids[:0]
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
