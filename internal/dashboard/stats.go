package dashboard

import (
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

// BuildStats constructs the dashboard stats response used by both the HTTP API
// and the Wails desktop binding.
func BuildStats(database *db.DB, calc *calculator.Calculator, deviceID string, source string, isPaused bool) (*model.StatsResponse, error) {
	now := time.Now().UTC()

	deviceID = normalizeDeviceID(deviceID)

	periods, err := buildPeriodCosts(database, now, deviceID, source)
	if err != nil {
		return nil, err
	}

	charts, err := buildCharts(database, now, deviceID, source)
	if err != nil {
		return nil, err
	}

	stats, err := database.QueryStatsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}
	sources := buildSourcesFromModelStats(stats, calc)

	deviceInfos, err := buildDeviceInfos(database)
	if err != nil {
		return nil, err
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}

	return &model.StatsResponse{
		Periods:  periods,
		Charts:   charts,
		Sources:  sources,
		Devices:  deviceInfos,
		Projects: projects,
		IsPaused: isPaused,
	}, nil
}

func BuildStatsSummary(database *db.DB, _ *calculator.Calculator, deviceID string, source string, isPaused bool) (*model.StatsResponse, error) {
	now := time.Now().UTC()
	deviceID = normalizeDeviceID(deviceID)

	periods, err := buildPeriodCosts(database, now, deviceID, source)
	if err != nil {
		return nil, err
	}

	charts, err := buildCharts(database, now, deviceID, source)
	if err != nil {
		return nil, err
	}

	stats, err := database.QuerySourceTotalsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}
	sources := make([]model.SourceStats, 0, len(stats))
	for _, stat := range stats {
		sources = append(sources, model.SourceStats{
			Name:               stat.Source,
			TotalInput:         stat.InputTokens,
			TotalCached:        stat.CachedTokens,
			TotalCacheCreation: stat.CacheCreationTokens,
			TotalOutput:        stat.OutputTokens,
			TotalCost:          stat.TotalCost,
			TotalEvents:        stat.Events,
			CacheHitRate:       model.CacheHitRatePercent(stat.InputTokens, stat.CachedTokens),
		})
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})

	deviceInfos, err := buildDeviceInfos(database)
	if err != nil {
		return nil, err
	}

	return &model.StatsResponse{
		Periods:  periods,
		Charts:   charts,
		Sources:  sources,
		Devices:  deviceInfos,
		IsPaused: isPaused,
	}, nil
}

func BuildStatsDetails(database *db.DB, calc *calculator.Calculator, deviceID string, source string, isPaused bool) (*model.StatsResponse, error) {
	deviceID = normalizeDeviceID(deviceID)

	stats, err := database.QueryStatsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}
	sources := buildSourcesFromModelStats(stats, calc)

	projects, err := database.QueryProjectStatsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}

	return &model.StatsResponse{
		Sources:  sources,
		Projects: projects,
		IsPaused: isPaused,
	}, nil
}

func normalizeDeviceID(deviceID string) string {
	if deviceID == "all" {
		return ""
	}
	return deviceID
}

func buildPeriodCosts(database *db.DB, now time.Time, deviceID string, source string) ([]model.PeriodCost, error) {
	windows := []struct {
		label string
		dur   time.Duration
	}{
		{"1h", 1 * time.Hour},
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"3mo", 90 * 24 * time.Hour},
		{"6mo", 180 * 24 * time.Hour},
		{"1y", 365 * 24 * time.Hour},
	}

	periodWindows := make([]db.PeriodStatsWindow, 0, len(windows)+1)
	for _, win := range windows {
		periodWindows = append(periodWindows, db.PeriodStatsWindow{
			Label: win.label,
			Since: now.Add(-win.dur),
		})
	}
	periodWindows = append(periodWindows, db.PeriodStatsWindow{Label: "ALL"})

	periodBuckets, err := database.QueryPeriodStatsBuckets(periodWindows, deviceID, source)
	if err != nil {
		return nil, err
	}
	periods := make([]model.PeriodCost, 0, len(periodBuckets))
	for _, bucket := range periodBuckets {
		periods = append(periods, model.PeriodCost{
			Label:               bucket.Label,
			Cost:                bucket.Cost,
			InputTokens:         bucket.InputTokens,
			CachedTokens:        bucket.CachedTokens,
			CacheCreationTokens: bucket.CacheCreationTokens,
			OutputTokens:        bucket.OutputTokens,
			CacheHitRate:        model.CacheHitRatePercent(bucket.InputTokens, bucket.CachedTokens),
		})
	}
	return periods, nil
}

func buildCharts(database *db.DB, now time.Time, deviceID string, source string) (model.StatsCharts, error) {
	const chartDays = 30

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -(chartDays - 1))
	end := start.AddDate(0, 0, chartDays)

	dailyCosts, err := database.QueryDailyCostBuckets(start, end, deviceID, source)
	if err != nil {
		return model.StatsCharts{}, err
	}
	heatmap, err := database.QueryCalendarHeatmapBuckets(start, end, deviceID, source)
	if err != nil {
		return model.StatsCharts{}, err
	}
	toolShare, err := database.QueryToolShareBuckets(start, end, deviceID, "")
	if err != nil {
		return model.StatsCharts{}, err
	}

	return model.StatsCharts{
		DailyCosts:      fillDailyCosts(start, chartDays, dailyCosts),
		CalendarHeatmap: fillCalendarHeatmap(start, chartDays, heatmap),
		ToolShare:       toolShare,
	}, nil
}

func fillDailyCosts(start time.Time, days int, buckets []model.DailyCostBucket) []model.DailyCostBucket {
	byDate := make(map[string]model.DailyCostBucket, len(buckets))
	for _, bucket := range buckets {
		byDate[bucket.Date] = bucket
	}

	filled := make([]model.DailyCostBucket, 0, days)
	for offset := 0; offset < days; offset++ {
		date := start.AddDate(0, 0, offset).Format("2006-01-02")
		bucket, ok := byDate[date]
		if !ok {
			bucket = model.DailyCostBucket{Date: date}
		}
		filled = append(filled, bucket)
	}
	return filled
}

func fillCalendarHeatmap(start time.Time, days int, buckets []model.CalendarHeatmapBucket) []model.CalendarHeatmapBucket {
	byDate := make(map[string]model.CalendarHeatmapBucket, len(buckets))
	for _, bucket := range buckets {
		byDate[bucket.Date] = bucket
	}

	filled := make([]model.CalendarHeatmapBucket, 0, days)
	for offset := 0; offset < days; offset++ {
		date := start.AddDate(0, 0, offset).Format("2006-01-02")
		bucket, ok := byDate[date]
		if !ok {
			bucket = model.CalendarHeatmapBucket{Date: date}
		}
		filled = append(filled, bucket)
	}
	return filled
}

func buildSourcesFromModelStats(stats []db.ModelStat, calc *calculator.Calculator) []model.SourceStats {
	sourceMap := make(map[string]*model.SourceStats)
	for _, s := range stats {
		src, ok := sourceMap[s.Source]
		if !ok {
			src = &model.SourceStats{Name: s.Source}
			sourceMap[s.Source] = src
		}
		price, _ := calc.GetModelPrice(s.Model)
		src.Models = append(src.Models, model.ModelStats{
			Model:                  s.Model,
			Events:                 s.Events,
			InputTokens:            s.InputTokens,
			CachedTokens:           s.CachedTokens,
			CacheCreationTokens:    s.CacheCreationTokens,
			OutputTokens:           s.OutputTokens,
			TotalCost:              s.TotalCost,
			CacheHitRate:           model.CacheHitRatePercent(s.InputTokens, s.CachedTokens),
			InputPricePerM:         price.InputPricePerM,
			CachedPricePerM:        price.CachedPricePerM,
			CacheCreationPricePerM: price.CacheCreationPricePerM,
			OutputPricePerM:        price.OutputPricePerM,
		})
		src.TotalInput += s.InputTokens
		src.TotalCached += s.CachedTokens
		src.TotalCacheCreation += s.CacheCreationTokens
		src.TotalOutput += s.OutputTokens
		src.TotalCost += s.TotalCost
		src.TotalEvents += s.Events
	}

	sources := make([]model.SourceStats, 0, len(sourceMap))
	for _, s := range sourceMap {
		s.CacheHitRate = model.CacheHitRatePercent(s.TotalInput, s.TotalCached)
		sources = append(sources, *s)
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})
	return sources
}

func buildDeviceInfos(database *db.DB) ([]model.DeviceInfo, error) {
	devices, err := database.QueryDevices()
	if err != nil {
		return nil, err
	}
	aliases, err := database.GetDeviceAliases()
	if err != nil {
		return nil, err
	}

	deviceInfos := make([]model.DeviceInfo, 0, len(devices))
	for _, id := range devices {
		name := id
		if alias, ok := aliases[id]; ok && alias != "" {
			name = alias
		}
		deviceInfos = append(deviceInfos, model.DeviceInfo{ID: id, DisplayName: name})
	}
	return deviceInfos, nil
}

// BuildTokenSummary constructs the lightweight aggregate advertised to LAN
// peers during discovery.
func BuildTokenSummary(database *db.DB, deviceID string) (model.TokenSummary, error) {
	deviceID = normalizeDeviceID(deviceID)

	now := time.Now().UTC()
	sources, err := database.QueryTokenSourceSummaries(now.Add(-24*time.Hour), deviceID)
	if err != nil {
		return model.TokenSummary{}, err
	}

	var tokens24h int
	var tokensTotal int
	var costTotal float64
	for _, source := range sources {
		tokens24h += source.Tokens24h
		tokensTotal += source.TokensTotal
		costTotal += source.CostTotal
	}

	return model.TokenSummary{
		Tokens24h:   tokens24h,
		TokensTotal: tokensTotal,
		CostTotal:   costTotal,
		Sources:     sources,
	}, nil
}
