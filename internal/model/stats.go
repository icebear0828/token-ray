package model

import "time"

// --- Shared types used by both web/handler.go and desktop/app.go ---

// ModelStats represents per-model aggregated usage and cost.
type ModelStats struct {
	Model                  string  `json:"model"`
	Events                 int     `json:"events"`
	InputTokens            int     `json:"input_tokens"`
	CachedTokens           int     `json:"cached_tokens"`
	CacheCreationTokens    int     `json:"cache_creation_tokens"`
	OutputTokens           int     `json:"output_tokens"`
	TotalCost              float64 `json:"total_cost"`
	CacheHitRate           float64 `json:"cache_hit_rate"`
	InputPricePerM         float64 `json:"input_price_per_m"`
	CachedPricePerM        float64 `json:"cached_price_per_m"`
	CacheCreationPricePerM float64 `json:"cache_creation_price_per_m"`
	OutputPricePerM        float64 `json:"output_price_per_m"`
}

// SourceStats represents per-source (e.g., "Claude Code") aggregated usage.
type SourceStats struct {
	Name               string       `json:"name"`
	TotalInput         int          `json:"total_input"`
	TotalCached        int          `json:"total_cached"`
	TotalCacheCreation int          `json:"total_cache_creation"`
	TotalOutput        int          `json:"total_output"`
	TotalCost          float64      `json:"total_cost"`
	TotalEvents        int          `json:"total_events"`
	CacheHitRate       float64      `json:"cache_hit_rate"`
	Models             []ModelStats `json:"models"`
}

// PeriodCost represents token usage and cost over a time period.
type PeriodCost struct {
	Label               string  `json:"label"`
	Cost                float64 `json:"cost"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
}

// DailyCostBucket represents one calendar day of spend.
type DailyCostBucket struct {
	Date string  `json:"date"`
	Cost float64 `json:"cost"`
}

// CalendarHeatmapBucket represents one GitHub-style calendar heatmap cell.
type CalendarHeatmapBucket struct {
	Date   string  `json:"date"`
	Tokens int     `json:"tokens"`
	Cost   float64 `json:"cost"`
	Events int     `json:"events"`
}

// ToolShareBucket represents token share by source/tool.
type ToolShareBucket struct {
	Source    string  `json:"source"`
	Tokens    int     `json:"tokens"`
	TotalCost float64 `json:"total_cost"`
	Events    int     `json:"events"`
}

// StatsCharts groups chart-ready aggregates for the dashboard summary.
type StatsCharts struct {
	DailyCosts      []DailyCostBucket       `json:"daily_costs"`
	CalendarHeatmap []CalendarHeatmapBucket `json:"calendar_heatmap"`
	ToolShare       []ToolShareBucket       `json:"tool_share"`
}

// DeviceInfo represents a device with its display name.
type DeviceInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// DeviceSummary represents a device row in the management UI.
type DeviceSummary struct {
	ID                  string    `json:"id"`
	DisplayName         string    `json:"display_name"`
	Events              int       `json:"events"`
	InputTokens         int       `json:"input_tokens"`
	CachedTokens        int       `json:"cached_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	TotalCost           float64   `json:"total_cost"`
	FirstSeen           time.Time `json:"first_seen,omitempty"`
	LastSeen            time.Time `json:"last_seen,omitempty"`
}

// DeviceSupersedeResponse describes a soft-delete operation for a device ID.
type DeviceSupersedeResponse struct {
	DeviceID        string `json:"device_id"`
	SupersededCount int64  `json:"superseded_count"`
}

// ProjectStat represents aggregated token usage and cost for a project.
type ProjectStat struct {
	Project             string  `json:"project"`
	Events              int     `json:"events"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalCost           float64 `json:"total_cost"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
}

// CacheHitRatePercent returns cached token share as a bounded 0-100 percentage.
func CacheHitRatePercent(inputTokens int, cachedTokens int) float64 {
	if inputTokens <= 0 || cachedTokens <= 0 {
		return 0
	}
	rate := (float64(cachedTokens) / float64(inputTokens)) * 100
	if rate > 100 {
		return 100
	}
	return rate
}

// StatsResponse is the full stats API response.
type StatsResponse struct {
	Periods  []PeriodCost  `json:"periods"`
	Charts   StatsCharts   `json:"charts"`
	Sources  []SourceStats `json:"sources"`
	Devices  []DeviceInfo  `json:"devices"`
	Projects []ProjectStat `json:"projects"`
	IsPaused bool          `json:"is_paused"`
}

// LANPeerInfo describes a discovered LAN peer and the local sync state for it.
type LANPeerInfo struct {
	ID              string               `json:"id"`
	DisplayName     string               `json:"display_name"`
	IP              string               `json:"ip"`
	HTTPPort        int                  `json:"http_port"`
	LastSeen        time.Time            `json:"last_seen"`
	LastSync        time.Time            `json:"last_sync"`
	LastSyncAttempt time.Time            `json:"last_sync_attempt"`
	SyncStatus      string               `json:"sync_status"`
	SyncError       string               `json:"sync_error"`
	Tokens24h       int                  `json:"tokens_24h"`
	TokensTotal     int                  `json:"tokens_total"`
	CostTotal       float64              `json:"cost_total"`
	Sources         []TokenSourceSummary `json:"sources,omitempty"`
}

// LANSelfResponse identifies the local node for active LAN discovery.
type LANSelfResponse struct {
	DeviceID string        `json:"device_id"`
	HTTPPort int           `json:"http_port"`
	Summary  *TokenSummary `json:"summary,omitempty"`
}

// LANScanResponse is returned by /api/lan/scan. Peers is kept for older
// clients; PeerInfos is the canonical structured payload.
type LANScanResponse struct {
	Peers     []string      `json:"peers"`
	PeerInfos []LANPeerInfo `json:"peer_infos"`
}

// LANStatusResponse is returned by LAN control endpoints.
type LANStatusResponse struct {
	Enabled bool `json:"enabled"`
}

// SystemLogsResponse points clients at the local stats/log directory.
type SystemLogsResponse struct {
	Path string `json:"path"`
}

// CacheSavingsResponse is the cache savings analysis response.
type CacheSavingsResponse struct {
	ActualCost       float64 `json:"actual_cost"`
	HypotheticalCost float64 `json:"hypothetical_cost"`
	Saved            float64 `json:"saved"`
	SavedPercent     float64 `json:"saved_percent"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
}
