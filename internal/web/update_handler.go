package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"ai-flight-dashboard/internal/updater"
)

const updateCheckTTL = 10 * time.Minute

type cachedUpdateCheck struct {
	at     time.Time
	status updater.UpdateStatus
}

var (
	updateCheckMu    sync.Mutex
	updateCheckCache = map[string]cachedUpdateCheck{}
)

func updateCheckCacheKey() string {
	return updater.GitHubAPIBase + "|" + updater.GitHubRepo + "|" + updater.CurrentVersion
}

func readCachedUpdateCheck(key string) (updater.UpdateStatus, bool) {
	updateCheckMu.Lock()
	defer updateCheckMu.Unlock()
	entry, ok := updateCheckCache[key]
	if !ok || time.Since(entry.at) > updateCheckTTL {
		return updater.UpdateStatus{}, false
	}
	return entry.status, true
}

func storeCachedUpdateCheck(key string, status updater.UpdateStatus) {
	updateCheckMu.Lock()
	defer updateCheckMu.Unlock()
	updateCheckCache[key] = cachedUpdateCheck{at: time.Now(), status: status}
}

func clearUpdateCheckCache() {
	updateCheckMu.Lock()
	defer updateCheckMu.Unlock()
	updateCheckCache = map[string]cachedUpdateCheck{}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func newUpdateClient() *updater.Client {
	return updater.NewClient(
		updater.WithBaseURL(updater.GitHubAPIBase),
		updater.WithRepo(updater.GitHubRepo),
		updater.WithToken(os.Getenv("GITHUB_TOKEN")),
	)
}

func handleSystemVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"version": updater.CurrentVersion,
		"goos":    runtime.GOOS,
		"goarch":  runtime.GOARCH,
	})
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cacheKey := updateCheckCacheKey()
	if status, ok := readCachedUpdateCheck(cacheKey); ok {
		writeJSON(w, http.StatusOK, status)
		return
	}

	status := updater.UpdateStatus{CurrentVersion: updater.CurrentVersion}
	release, err := newUpdateClient().CheckForUpdates(updater.CurrentVersion)
	if err != nil {
		status.HasUpdate = false
		status.LatestVersion = updater.CurrentVersion
		status.Error = err.Error()
		writeJSON(w, http.StatusOK, status)
		return
	}

	if release == nil {
		status.LatestVersion = updater.CurrentVersion
		status.HasUpdate = false
	} else {
		status.HasUpdate = true
		status.LatestVersion = release.TagName
		status.ReleaseNotes = release.Body
		if !release.PublishedAt.IsZero() {
			status.PublishedAt = release.PublishedAt.Format(time.RFC3339)
		}
		if asset := updater.MatchAsset(release.Assets, runtime.GOOS, runtime.GOARCH); asset != nil {
			status.AssetName = asset.Name
			status.DownloadURL = asset.URL
			if status.DownloadURL == "" {
				status.DownloadURL = asset.BrowserDownloadURL
			}
		}
	}

	storeCachedUpdateCheck(cacheKey, status)
	writeJSON(w, http.StatusOK, status)
}

func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "Method not allowed",
		})
		return
	}

	client := newUpdateClient()
	release, err := client.CheckForUpdates(updater.CurrentVersion)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"message": fmt.Sprintf("Failed to check release: %v", err),
		})
		return
	}
	if release == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"message": "Already running latest version (" + updater.CurrentVersion + ")",
		})
		return
	}

	if err := client.ApplyUpdate(release); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": fmt.Sprintf("Failed to apply update: %v", err),
		})
		return
	}

	// The on-disk binary changed; drop cached check data so the next check is fresh.
	clearUpdateCheckCache()

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Successfully updated to " + release.TagName + ". Please restart the process to apply changes.",
		"version": release.TagName,
	})
}
