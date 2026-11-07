package web

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"ai-flight-dashboard/internal/updater"
)

func handleSystemVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
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

	token := os.Getenv("GITHUB_TOKEN")
	client := updater.NewClient(
		updater.WithBaseURL(updater.GitHubAPIBase),
		updater.WithRepo(updater.GitHubRepo),
		updater.WithToken(token),
	)

	release, err := client.CheckForUpdates(updater.CurrentVersion)
	status := updater.UpdateStatus{
		CurrentVersion: updater.CurrentVersion,
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		status.HasUpdate = false
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": status,
			"error":  err.Error(),
		})
		return
	}

	if release != nil {
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
	} else {
		status.LatestVersion = updater.CurrentVersion
		status.HasUpdate = false
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := os.Getenv("GITHUB_TOKEN")
	client := updater.NewClient(
		updater.WithBaseURL(updater.GitHubAPIBase),
		updater.WithRepo(updater.GitHubRepo),
		updater.WithToken(token),
	)

	// Fetch latest release
	release, err := client.CheckForUpdates(updater.CurrentVersion)
	if err != nil {
		http.Error(w, "Failed to check release: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if release == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Already running latest version (" + updater.CurrentVersion + ")",
		})
		return
	}

	// Apply update
	if err := client.ApplyUpdate(release); err != nil {
		http.Error(w, "Failed to apply update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Successfully updated to " + release.TagName + ". Please restart the process to apply changes.",
		"version": release.TagName,
	})
}
