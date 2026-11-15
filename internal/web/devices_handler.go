package web

import (
	"token-ray/internal/db"
	"token-ray/internal/model"
	"encoding/json"
	"net/http"
	"strings"
)

func handleDevices(w http.ResponseWriter, r *http.Request, database *db.DB, token string) {
	if r.Method == http.MethodGet {
		devices, err := database.QueryDeviceSummaries()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(devices)
		return
	}

	if r.Method == http.MethodDelete {
		authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
			deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
			if deviceID == "" {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			changed, err := database.SupersedeDevice(deviceID)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(model.DeviceSupersedeResponse{
				DeviceID:        deviceID,
				SupersededCount: changed,
			})
		})(w, r)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleDeviceAlias(w http.ResponseWriter, r *http.Request, database *db.DB) {
	if r.Method == http.MethodPost {
		var req struct {
			DeviceID    string `json:"device_id"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.DeviceID) == "" || strings.TrimSpace(req.DisplayName) == "" {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if err := database.SetDeviceAlias(req.DeviceID, req.DisplayName); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == http.MethodDelete {
		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		if deviceID == "" {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if _, err := database.DeleteDeviceAlias(deviceID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
