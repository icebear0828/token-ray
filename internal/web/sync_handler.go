package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"token-ray/internal/db"
)

const (
	defaultSyncLimit = 1000
	maxSyncLimit     = 5000
)

func handleSyncPull(w http.ResponseWriter, r *http.Request, database *db.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sinceStr := r.URL.Query().Get("since")
	var since time.Time
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339Nano, sinceStr)
		if err != nil {
			http.Error(w, "Invalid since format, expected RFC3339", http.StatusBadRequest)
			return
		}
		since = t
	}

	var afterID int64
	if afterStr := r.URL.Query().Get("after_id"); afterStr != "" {
		id, err := strconv.ParseInt(afterStr, 10, 64)
		if err != nil || id < 0 {
			http.Error(w, "Invalid after_id", http.StatusBadRequest)
			return
		}
		afterID = id
	}

	limit := defaultSyncLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	if limit > maxSyncLimit {
		limit = maxSyncLimit
	}

	deviceID := r.URL.Query().Get("device_id")
	page, err := database.QuerySyncRecordsPageForDevice(since, afterID, limit, deviceID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
