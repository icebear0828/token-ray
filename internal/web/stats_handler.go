package web

import (
	"encoding/json"
	"net/http"
	"strings"

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

	payload, etag, err := statsCache.GetJSON(dashboard.StatsCacheKey{
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

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if ifNoneMatchMatches(r.Header.Values("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload)
}

func ifNoneMatchMatches(headers []string, currentETag string) bool {
	if currentETag == "" {
		return false
	}
	target := stripWeakPrefix(strings.TrimSpace(currentETag))
	for _, header := range headers {
		for _, candidate := range splitETagList(header) {
			trimmed := strings.TrimSpace(candidate)
			if trimmed == "" {
				continue
			}
			if trimmed == "*" {
				return true
			}
			if stripWeakPrefix(trimmed) == target {
				return true
			}
		}
	}
	return false
}

func stripWeakPrefix(tag string) string {
	return strings.TrimPrefix(tag, "W/")
}

// splitETagList splits comma-separated entity-tags while respecting quoted strings.
func splitETagList(header string) []string {
	var tags []string
	start := 0
	inQuotes := false
	escaped := false
	for i := 0; i < len(header); i++ {
		c := header[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inQuotes {
			escaped = true
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if c == ',' && !inQuotes {
			tags = append(tags, header[start:i])
			start = i + 1
		}
	}
	tags = append(tags, header[start:])
	return tags
}

func handleCacheSavings(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator) {
	savings, err := dashboard.BuildCacheSavings(database, calc, r.URL.Query().Get("device"))
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(savings)
}
