package agyscanner

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"ai-flight-dashboard/internal/watcher"
)

var logPortRe = regexp.MustCompile(`listening on random port at (\d+) for HTTP$`)

// Backfill resumes each historical agy conversation via `agy --print`,
// discovers the temporary HTTP port, and scans the trajectory for token data.
// Returns the total number of new records inserted.
func (s *Scanner) Backfill(usageChan chan<- watcher.TokenUsage) (int, error) {
	convIDs := s.unscannedConversations()
	if len(convIDs) == 0 {
		return 0, nil
	}

	workspaces := s.loadWorkspaces()
	total := 0

	for i, convID := range convIDs {
		log.Printf("[agy backfill] (%d/%d) resuming conversation %s", i+1, len(convIDs), convID)

		n, err := s.backfillConversation(convID, workspaces, usageChan)
		if err != nil {
			log.Printf("[agy backfill] conversation %s failed: %v", convID, err)
			continue
		}
		total += n
		if n > 0 {
			log.Printf("[agy backfill] conversation %s: %d records", convID, n)
		}
	}
	return total, nil
}

func (s *Scanner) backfillConversation(convID string, workspaces map[string]string, usageChan chan<- watcher.TokenUsage) (int, error) {
	logFile := filepath.Join(os.TempDir(), fmt.Sprintf("agy-backfill-%s.log", convID))
	defer os.Remove(logFile)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	agyPath := findAgyBinary()
	if agyPath == "" {
		return 0, fmt.Errorf("agy binary not found")
	}

	cmd := exec.CommandContext(ctx, agyPath,
		"--conversation", convID,
		"--print", "ok",
		"--log-file", logFile,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start agy: %w", err)
	}

	port, err := waitForPort(ctx, logFile)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return 0, fmt.Errorf("port discovery: %w", err)
	}

	n := 0
	summaries, err := getAllTrajectories(port)
	if err != nil {
		log.Printf("[agy backfill] getAllTrajectories on port %d failed: %v", port, err)
	} else if len(summaries) == 0 {
		// No trajectories listed — try using convID as cascade_id directly
		count, scanErr := s.scanTrajectory(port, convID, workspaces, usageChan)
		if scanErr != nil {
			log.Printf("[agy backfill] direct scanTrajectory(%s) failed: %v", convID, scanErr)
		} else {
			n = count
		}
	} else {
		for cascadeID := range summaries {
			count, scanErr := s.scanTrajectory(port, cascadeID, workspaces, usageChan)
			if scanErr != nil {
				continue
			}
			n += count
		}
	}

	cmd.Wait()
	return n, nil
}

func waitForPort(ctx context.Context, logPath string) (int, error) {
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		port := parsePortFromLog(logPath)
		if port > 0 {
			if isPortAlive(port) {
				return port, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func parsePortFromLog(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := logPortRe.FindStringSubmatch(sc.Text())
		if m != nil {
			port, _ := strconv.Atoi(m[1])
			return port
		}
	}
	return 0
}

func (s *Scanner) unscannedConversations() []string {
	ws := s.loadWorkspaces()
	var unscanned []string
	for convID := range ws {
		offsetKey := "agy:" + convID + ":gmCount"
		count, _ := s.db.GetOffset(offsetKey)
		if count == 0 {
			unscanned = append(unscanned, convID)
		}
	}
	return unscanned
}

func findAgyBinary() string {
	if path, err := exec.LookPath("agy"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	local := filepath.Join(home, ".local", "bin", "agy")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}
