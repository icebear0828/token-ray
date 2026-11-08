package dashboard

import (
	"sort"
	"strings"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

// periodDurations maps the dashboard time-window labels to their lookback
// duration. ALL has no lookback and is handled separately.
var periodDurations = map[string]time.Duration{
	"1h":  1 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"3mo": 90 * 24 * time.Hour,
	"6mo": 180 * 24 * time.Hour,
	"1y":  365 * 24 * time.Hour,
}

// NormalizePeriod normalizes an incoming period query value. The boolean is
// false when the value is not a recognized window. Empty input means ALL so
// older clients keep working.
func NormalizePeriod(period string) (string, bool) {
	trimmed := strings.TrimSpace(period)
	if trimmed == "" {
		return "ALL", true
	}
	lower := strings.ToLower(trimmed)
	if lower == "all" {
		return "ALL", true
	}
	if _, ok := periodDurations[lower]; ok {
		return lower, true
	}
	return "ALL", false
}

// CanonicalPeriod returns the normalized period, falling back to ALL.
func CanonicalPeriod(period string) string {
	canonical, _ := NormalizePeriod(period)
	return canonical
}

// PeriodSince returns the lookback boundary for a period. A zero time means
// all-time (no lower bound).
func PeriodSince(now time.Time, period string) time.Time {
	duration, ok := periodDurations[CanonicalPeriod(period)]
	if !ok {
		return time.Time{}
	}
	return now.Add(-duration)
}

// BuildStats constructs the dashboard stats response used by both the HTTP API
// and the Wails desktop binding.
func BuildStats(database *db.DB, calc *calculator.Calculator, deviceID string, source string, period string, isPaused bool) (*model.StatsResponse, error) {
	now := time.Now().UTC()

	deviceID = normalizeDeviceID(deviceID)
	period = CanonicalPeriod(period)
	since := PeriodSince(now, period)

	periods, err := buildPeriodCosts(database, now, deviceID, source)
	if err != nil {
		return nil, err
	}

	charts, err := buildCharts(database, now, deviceID, source, period)
	if err != nil {
		return nil, err
	}

	stats, err := database.QueryStatsSince(since, deviceID, source)
	if err != nil {
		return nil, err
	}
	sources := buildSourcesFromModelStats(stats, calc)

	deviceInfos, err := buildDeviceInfos(database)
	if err != nil {
		return nil, err
	}

	projects, err := database.QueryProjectStatsSince(since, deviceID, source)
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

func BuildStatsSummary(database *db.DB, _ *calculator.Calculator, deviceID string, source string, period string, isPaused bool) (*model.StatsResponse, error) {
	now := time.Now().UTC()
	deviceID = normalizeDeviceID(deviceID)
	period = CanonicalPeriod(period)
	since := PeriodSince(now, period)

	periods, err := buildPeriodCosts(database, now, deviceID, source)
	if err != nil {
		return nil, err
	}

	charts, err := buildCharts(database, now, deviceID, source, period)
	if err != nil {
		return nil, err
	}

	stats, err := database.QuerySourceTotalsSince(since, deviceID, source)
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

func BuildStatsDetails(database *db.DB, calc *calculator.Calculator, deviceID string, source string, period string, isPaused bool) (*model.StatsResponse, error) {
	deviceID = normalizeDeviceID(deviceID)
	period = CanonicalPeriod(period)
	since := PeriodSince(time.Now().UTC(), period)

	stats, err := database.QueryStatsSince(since, deviceID, source)
	if err != nil {
		return nil, err
	}
	sources := buildSourcesFromModelStats(stats, calc)

	projects, err := database.QueryProjectStatsSince(since, deviceID, source)
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

func buildCharts(database *db.DB, now time.Time, deviceID string, source string, period string) (model.StatsCharts, error) {
	granularity, start, end, count := chartWindow(now, period)

	costBuckets, err := database.QueryDailyCostBuckets(start, end, granularity, deviceID, source)
	if err != nil {
		return model.StatsCharts{}, err
	}

	// The calendar heatmap and source-share donut keep their fixed 30-day
	// window; they are activity overviews rather than window-scoped trends.
	const chartDays = 30
	today := startOfDay(now)
	heatmapStart := today.AddDate(0, 0, -(chartDays - 1))
	heatmapEnd := heatmapStart.AddDate(0, 0, chartDays)

	heatmap, err := database.QueryCalendarHeatmapBuckets(heatmapStart, heatmapEnd, deviceID, source)
	if err != nil {
		return model.StatsCharts{}, err
	}
	toolShare, err := database.QueryToolShareBuckets(heatmapStart, heatmapEnd, deviceID, "")
	if err != nil {
		return model.StatsCharts{}, err
	}

	return model.StatsCharts{
		Granularity:     granularity,
		DailyCosts:      fillCostBuckets(granularity, start, end, count, costBuckets),
		CalendarHeatmap: fillCalendarHeatmap(heatmapStart, chartDays, heatmap),
		ToolShare:       toolShare,
	}, nil
}

// chartWindow resolves the bucket granularity and range for a period. count is
// the number of buckets to emit; zero means "derive from the earliest record"
// (all-time).
func chartWindow(now time.Time, period string) (granularity string, start time.Time, end time.Time, count int) {
	switch CanonicalPeriod(period) {
	case "1h":
		granularity = db.BucketHourly
		end = now.Truncate(time.Hour).Add(time.Hour)
		// Two hourly buckets so the current (partial) hour is shown alongside
		// the previous hour rather than a single misleading partial bar.
		count = 2
	case "24h":
		granularity = db.BucketHourly
		end = now.Truncate(time.Hour).Add(time.Hour)
		count = 24
	case "7d":
		granularity = db.BucketDaily
		end = startOfDay(now).AddDate(0, 0, 1)
		count = 7
	case "30d":
		granularity = db.BucketDaily
		end = startOfDay(now).AddDate(0, 0, 1)
		count = 30
	case "3mo":
		granularity = db.BucketDaily
		end = startOfDay(now).AddDate(0, 0, 1)
		count = 90
	case "6mo":
		granularity = db.BucketMonthly
		end = startOfMonth(now).AddDate(0, 1, 0)
		count = 6
	case "1y":
		granularity = db.BucketMonthly
		end = startOfMonth(now).AddDate(0, 1, 0)
		count = 12
	default:
		granularity = db.BucketMonthly
		end = startOfMonth(now).AddDate(0, 1, 0)
		count = 0
	}

	if count > 0 {
		start = advanceBucket(granularity, end, -count)
	}
	return granularity, start, end, count
}

func fillCostBuckets(granularity string, start time.Time, end time.Time, count int, buckets []model.DailyCostBucket) []model.DailyCostBucket {
	byDate := make(map[string]model.DailyCostBucket, len(buckets))
	for _, bucket := range buckets {
		byDate[bucket.Date] = bucket
	}

	if count == 0 {
		if len(buckets) == 0 {
			return []model.DailyCostBucket{}
		}
		start = parseBucketStart(granularity, buckets[0].Date)
		if start.IsZero() {
			return buckets
		}
	}

	filled := make([]model.DailyCostBucket, 0, max(count, 1))
	for cursor := start; cursor.Before(end); cursor = nextBucketStart(granularity, cursor) {
		key := formatBucketKey(granularity, cursor)
		bucket, ok := byDate[key]
		if !ok {
			bucket = model.DailyCostBucket{Date: key}
		}
		filled = append(filled, bucket)
		if count > 0 && len(filled) >= count {
			break
		}
	}
	return filled
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func nextBucketStart(granularity string, t time.Time) time.Time {
	switch granularity {
	case db.BucketHourly:
		return t.Add(time.Hour)
	case db.BucketMonthly:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}

// advanceBucket moves t by n buckets (n may be negative).
func advanceBucket(granularity string, t time.Time, n int) time.Time {
	switch granularity {
	case db.BucketHourly:
		return t.Add(time.Duration(n) * time.Hour)
	case db.BucketMonthly:
		return t.AddDate(0, n, 0)
	default:
		return t.AddDate(0, 0, n)
	}
}

func formatBucketKey(granularity string, t time.Time) string {
	switch granularity {
	case db.BucketHourly:
		return t.Format("2006-01-02 15:00")
	case db.BucketMonthly:
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02")
	}
}

func parseBucketStart(granularity string, value string) time.Time {
	var layout string
	switch granularity {
	case db.BucketHourly:
		layout = "2006-01-02 15:00"
	case db.BucketMonthly:
		layout = "2006-01"
	default:
		layout = "2006-01-02"
	}
	t, err := time.Parse(layout, value)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
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
