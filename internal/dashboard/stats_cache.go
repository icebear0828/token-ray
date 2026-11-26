package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"ai-flight-dashboard/internal/model"
)

type StatsCacheKey struct {
	DeviceID string
	Source   string
	Period   string
	Detail   string
	IsPaused bool
}

type StatsCache struct {
	ttl        time.Duration
	now        func() time.Time
	mu         sync.Mutex
	generation uint64
	entries    map[StatsCacheKey]statsCacheEntry
	flights    map[StatsCacheKey]*statsCacheFlight
}

type statsCacheEntry struct {
	stats     *model.StatsResponse
	jsonBytes []byte
	etag      string
	expiresAt time.Time
}

type statsCacheFlight struct {
	done      chan struct{}
	gen       uint64
	stats     *model.StatsResponse
	jsonBytes []byte
	etag      string
	err       error
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
	flight := &statsCacheFlight{done: make(chan struct{}), gen: c.generation}
	c.flights[key] = flight
	c.mu.Unlock()

	stats, err := build()
	cloned := cloneStatsResponse(stats)

	c.mu.Lock()
	if err == nil && c.generation == flight.gen {
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

// GetJSON returns cached immutable marshaled JSON bytes and a quoted strong
// ETag for key, building and coalescing concurrent misses. Callers must not
// modify the returned byte slice.
func (c *StatsCache) GetJSON(key StatsCacheKey, build func() (*model.StatsResponse, error)) ([]byte, string, error) {
	if c == nil || c.ttl <= 0 {
		stats, err := build()
		if err != nil {
			return nil, "", err
		}
		return marshalStatsJSON(stats)
	}

	now := c.now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) {
		if len(entry.jsonBytes) > 0 {
			raw, etag := entry.jsonBytes, entry.etag
			c.mu.Unlock()
			return raw, etag, nil
		}
		// Warm struct populated by Get(); marshal once outside lock and backfill.
		cachedStats := entry.stats
		gen := c.generation
		c.mu.Unlock()
		raw, etag, err := marshalStatsJSON(cachedStats)
		if err != nil {
			return nil, "", err
		}
		c.mu.Lock()
		if c.generation == gen {
			if cur, stillThere := c.entries[key]; stillThere && len(cur.jsonBytes) == 0 {
				cur.jsonBytes = raw
				cur.etag = etag
				c.entries[key] = cur
			}
		}
		c.mu.Unlock()
		return raw, etag, nil
	}
	if flight, ok := c.flights[key]; ok {
		c.mu.Unlock()
		<-flight.done
		if flight.err != nil {
			return nil, "", flight.err
		}
		if len(flight.jsonBytes) > 0 {
			return flight.jsonBytes, flight.etag, nil
		}
		return marshalStatsJSON(flight.stats)
	}
	flight := &statsCacheFlight{done: make(chan struct{}), gen: c.generation}
	c.flights[key] = flight
	c.mu.Unlock()

	stats, err := build()
	var cloned *model.StatsResponse
	var raw []byte
	var etag string
	if err == nil {
		cloned = cloneStatsResponse(stats)
		raw, etag, err = marshalStatsJSON(cloned)
	}

	c.mu.Lock()
	if err == nil && c.generation == flight.gen {
		c.entries[key] = statsCacheEntry{
			stats:     cloneStatsResponse(cloned),
			jsonBytes: raw,
			etag:      etag,
			expiresAt: c.now().Add(c.ttl),
		}
	}
	flight.stats = cloned
	flight.jsonBytes = raw
	flight.etag = etag
	flight.err = err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()

	if err != nil {
		return nil, "", err
	}
	return raw, etag, nil
}

func marshalStatsJSON(stats *model.StatsResponse) ([]byte, string, error) {
	raw, err := json.Marshal(stats)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	var etagBuf [34]byte
	etagBuf[0] = '"'
	hex.Encode(etagBuf[1:33], sum[:16])
	etagBuf[33] = '"'
	return raw, string(etagBuf[:]), nil
}

func (c *StatsCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.entries = make(map[StatsCacheKey]statsCacheEntry)
}

func cloneStatsResponse(stats *model.StatsResponse) *model.StatsResponse {
	if stats == nil {
		return nil
	}
	clone := *stats
	clone.Periods = append([]model.PeriodCost(nil), stats.Periods...)
	clone.Charts = model.StatsCharts{
		Granularity:     stats.Charts.Granularity,
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
