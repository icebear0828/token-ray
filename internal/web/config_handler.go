package web

import (
	"encoding/json"
	"net/http"
	"os"

	"token-ray/internal/config"
	"token-ray/internal/watcher"
)

// configSavedHook, when set, is invoked after a successful PUT /api/config so
// that runtime components (e.g. the LAN discovery instance) can refresh their
// view of the persisted config. main.go wires it during startup.
var configSavedHook func(*config.AppConfig)

// SetConfigSavedHook installs the post-save callback. Pass nil to clear.
func SetConfigSavedHook(h func(*config.AppConfig)) {
	configSavedHook = h
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, "Failed to load config", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func handlePutConfig(w http.ResponseWriter, r *http.Request, wInst *watcher.Watcher) {
	var cfg config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	oldCfg, _ := config.LoadConfig()

	if err := config.SaveConfig(&cfg); err != nil {
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	if wInst != nil {
		if oldCfg != nil {
			for _, oldDir := range oldCfg.ExtraWatchDirs {
				found := false
				for _, newDir := range cfg.ExtraWatchDirs {
					if oldDir == newDir {
						found = true
						break
					}
				}
				if !found {
					wInst.UnwatchDir(oldDir)
				}
			}
		}

		for _, dir := range cfg.ExtraWatchDirs {
			if _, err := os.Stat(dir); err == nil {
				wInst.WatchDirRecursive(dir)
			}
		}
	}

	if configSavedHook != nil {
		configSavedHook(&cfg)
	}

	w.WriteHeader(http.StatusOK)
}
