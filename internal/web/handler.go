package web

import (
	"embed"
	"io/fs"
	"net/http"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/watcher"
)

func NewHandler(database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher, lanInst *lan.LAN, token string, distBinFS embed.FS) http.Handler {
	return NewHandlerWithLANController(database, calc, wInst, newStaticLANController(lanInst), token, distBinFS)
}

func NewHandlerWithLANController(database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher, lanControl LANController, token string, distBinFS embed.FS) http.Handler {
	mux := http.NewServeMux()
	statsCache := dashboard.NewStatsCache(2 * time.Second)

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		handleStats(w, r, database, calc, wInst, statsCache)
	})

	mux.HandleFunc("/api/cache-savings", func(w http.ResponseWriter, r *http.Request) {
		handleCacheSavings(w, r, database, calc)
	})

	mux.HandleFunc("/api/pricing", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleGetPricing(w, r, calc)
		} else if r.Method == http.MethodPut || r.Method == http.MethodPost {
			authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
				handlePutPricing(w, r, database, calc, statsCache)
			})(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleGetConfig(w, r)
		} else if r.Method == http.MethodPut {
			authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
				handlePutConfig(w, r, wInst)
			})(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/track", authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handleTrack(w, r, database, calc, wInst)
	}))

	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		handleDevices(w, r, database, token)
	})

	mux.HandleFunc("/api/device-alias", authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handleDeviceAlias(w, r, database)
	}))

	mux.HandleFunc("/api/lan/self", func(w http.ResponseWriter, r *http.Request) {
		handleLANSelf(w, r, currentLAN(lanControl))
	})
	mux.HandleFunc("/api/lan/scan", func(w http.ResponseWriter, r *http.Request) {
		handleLANScan(w, r, database, currentLAN(lanControl))
	})
	mux.HandleFunc("/api/lan/status", func(w http.ResponseWriter, r *http.Request) {
		handleLANStatus(w, r, lanControl)
	})
	mux.HandleFunc("/api/lan/join", func(w http.ResponseWriter, r *http.Request) {
		handleLANJoin(w, r, lanControl)
	})
	mux.HandleFunc("/api/lan/leave", func(w http.ResponseWriter, r *http.Request) {
		handleLANLeave(w, r, lanControl)
	})

	mux.HandleFunc("/api/system/logs", handleSystemLogs)
	mux.HandleFunc("/api/system/version", handleSystemVersion)
	mux.HandleFunc("/api/system/update/check", handleUpdateCheck)
	mux.HandleFunc("/api/system/update/apply", authMiddleware(token, handleUpdateApply))

	mux.HandleFunc("/api/sync/pull", syncAuthMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handleSyncPull(w, r, database)
	}))

	mux.HandleFunc("/api/pause", authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handlePause(w, r, wInst)
	}))

	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		handleDownload(w, r, distBinFS)
	})
	mux.HandleFunc("/install.sh", handleInstallScript)

	staticFS, err := fs.Sub(StaticFiles, "dist")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return mux
}
