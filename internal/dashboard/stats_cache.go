package dashboard

import (
	"sync"
	"time"

	"ai-flight-dashboard/internal/model"
)

type StatsCacheKey struct {
	DeviceID string
	Source   string
	Detail   string
	IsPaused bool
}

type StatsCache struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	entries map[StatsCacheKey]statsCacheEntry
	flights map[StatsCacheKey]*statsCacheFlight
}

type statsCacheEntry struct {
	stats     *model.StatsResponse
	expiresAt time.Time
}

type statsCacheFlight struct {
	done  chan struct{}
	stats *model.StatsResponse
	err   error
}

func NewStatsCache(ttl time.Duration) *StatsCache {
	return NewStatsCacheWithClock(ttl, time.Now)
}

func NewStatsCacheWithClock(ttl time.Duration, now func() time.Time) *StatsCache {
	return &StatsCache{
		ttl:     ttl,
		now:     now,
		entries: make(map[StatsCacheKey]statsCacheEntry),
		flights: make(map[StatsCacheKey]*statsCacheFlight),
	}
}

func (c *StatsCache) Get(key StatsCacheKey, build func() (*model.StatsResponse, error)) (*model.StatsResponse, error) {
	if c == nil || c.ttl <= 0 {
		stats, err := build()
		return cloneStatsResponse(stats), err
	}

	now := c.now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) {
		stats := cloneStatsResponse(entry.stats)
		c.mu.Unlock()
		return stats, nil
	}
	if flight, ok := c.flights[key]; ok {
		c.mu.Unlock()
		<-flight.done
		return cloneStatsResponse(flight.stats), flight.err
	}
	flight := &statsCacheFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	stats, err := build()
	cloned := cloneStatsResponse(stats)

	c.mu.Lock()
	if err == nil {
		c.entries[key] = statsCacheEntry{
			stats:     cloneStatsResponse(cloned),
			expiresAt: c.now().Add(c.ttl),
		}
	}
	flight.stats = cloned
	flight.err = err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()

	return cloneStatsResponse(cloned), err
}

func (c *StatsCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[StatsCacheKey]statsCacheEntry)
}

func cloneStatsResponse(stats *model.StatsResponse) *model.StatsResponse {
	if stats == nil {
		return nil
	}
	clone := *stats
	clone.Periods = append([]model.PeriodCost(nil), stats.Periods...)
	clone.Charts = model.StatsCharts{
		DailyCosts:      append([]model.DailyCostBucket(nil), stats.Charts.DailyCosts...),
		CalendarHeatmap: append([]model.CalendarHeatmapBucket(nil), stats.Charts.CalendarHeatmap...),
		ToolShare:       append([]model.ToolShareBucket(nil), stats.Charts.ToolShare...),
	}
	clone.Devices = append([]model.DeviceInfo(nil), stats.Devices...)
	clone.Projects = append([]model.ProjectStat(nil), stats.Projects...)
	clone.Sources = make([]model.SourceStats, len(stats.Sources))
	for i, source := range stats.Sources {
		clone.Sources[i] = source
		clone.Sources[i].Models = append([]model.ModelStats(nil), source.Models...)
	}
	return &clone
}
