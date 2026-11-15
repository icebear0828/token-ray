package web

import (
	"encoding/json"
	"net/http"

	"token-ray/internal/calculator"
	"token-ray/internal/db"
	"token-ray/internal/model"
	"token-ray/internal/watcher"
)

func handleTrack(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if wInst != nil && wInst.IsPaused() {
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload model.TrackPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	cost, err := calc.CalculateCost(payload.Usage.Model, payload.Usage.InputTokens, payload.Usage.CachedTokens, payload.Usage.CacheCreationTokens, payload.Usage.OutputTokens)
	if err != nil {
		cost = 0
	}
	if err := database.InsertUsage(payload.Usage, cost, payload.DeviceID); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func handlePause(w http.ResponseWriter, r *http.Request, wInst *watcher.Watcher) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if wInst != nil {
		wInst.SetPaused(!wInst.IsPaused())
	}

	isPaused := false
	if wInst != nil {
		isPaused = wInst.IsPaused()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"is_paused": isPaused})
}
