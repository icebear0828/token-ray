package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"token-ray/internal/config"
	"token-ray/internal/model"
)

func handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logPath := filepath.Join(config.GetDataDir(), "stats")
	if err := os.MkdirAll(logPath, 0755); err != nil {
		http.Error(w, "Failed to prepare system logs directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(model.SystemLogsResponse{Path: logPath})
}
