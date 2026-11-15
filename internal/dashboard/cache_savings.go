package dashboard

import (
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

// BuildCacheSavings compares actual spend against a hypothetical where cached
// tokens were billed at the full input price.
func BuildCacheSavings(database *db.DB, calc *calculator.Calculator, deviceID string) (*model.CacheSavingsResponse, error) {
	records, err := database.QueryUsageRecords(time.Time{}, normalizeDeviceID(deviceID))
	if err != nil {
		return nil, err
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

	return &model.CacheSavingsResponse{
		ActualCost:       actualTotal,
		HypotheticalCost: hypoTotal,
		Saved:            saved,
		SavedPercent:     savedPct,
		CacheHitRate:     model.CacheHitRatePercent(totalInput, totalCached),
	}, nil
}
