package web

import (
	"encoding/json"
	"net/http"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

func handleStats(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher, statsCache *dashboard.StatsCache) {
	deviceID := r.URL.Query().Get("device")
	source := r.URL.Query().Get("source")
	detail := r.URL.Query().Get("detail")

	period, ok := dashboard.NormalizePeriod(r.URL.Query().Get("period"))
	if !ok {
		http.Error(w, "invalid stats period", http.StatusBadRequest)
		return
	}

	isPaused := false
	if wInst != nil {
		isPaused = wInst.IsPaused()
	}

	var build func() (*model.StatsResponse, error)
	switch detail {
	case "", "full":
		build = func() (*model.StatsResponse, error) {
			return dashboard.BuildStats(database, calc, deviceID, source, period, isPaused)
		}
	case "summary":
		build = func() (*model.StatsResponse, error) {
			return dashboard.BuildStatsSummary(database, calc, deviceID, source, period, isPaused)
		}
	case "details":
		build = func() (*model.StatsResponse, error) {
			return dashboard.BuildStatsDetails(database, calc, deviceID, source, period, isPaused)
		}
	default:
		http.Error(w, "invalid stats detail mode", http.StatusBadRequest)
		return
	}

	stats, err := statsCache.Get(dashboard.StatsCacheKey{
		DeviceID: deviceID,
		Source:   source,
		Period:   period,
		Detail:   detail,
		IsPaused: isPaused,
	}, build)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleCacheSavings(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator) {
	deviceID := r.URL.Query().Get("device")

	records, err := database.QueryUsageRecords(time.Time{}, deviceID)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	var actualTotal, hypoTotal float64
	var totalInput, totalCached int

	for _, rec := range records {
		actual, _ := calc.CalculateCost(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		hypo, _ := calc.CalculateCostNoCaching(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		actualTotal += actual
		hypoTotal += hypo
		totalInput += rec.InputTokens
		totalCached += rec.CachedTokens
	}

	saved := hypoTotal - actualTotal
	savedPct := 0.0
	if hypoTotal > 0 {
		savedPct = (saved / hypoTotal) * 100
	}
	hitRate := model.CacheHitRatePercent(totalInput, totalCached)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.CacheSavingsResponse{
		ActualCost:       actualTotal,
		HypotheticalCost: hypoTotal,
		Saved:            saved,
		SavedPercent:     savedPct,
		CacheHitRate:     hitRate,
	})
}
