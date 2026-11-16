package model

import "time"

// TokenUsage represents a single AI tool usage event with token counts.
// Shared by watcher, scanner, db, and tui packages.
type TokenUsage struct {
	DeviceID            string `json:"device_id,omitempty"` // For LAN routing
	Source              string `json:"source"`
	Model               string `json:"model"`
	Project             string `json:"project"`
	InputTokens         int    `json:"input_tokens"`
	CachedTokens        int    `json:"cached_tokens"`
	CacheCreationTokens int    `json:"cache_creation_tokens"`
	OutputTokens        int    `json:"output_tokens"`
	Thoughts            int    `json:"thoughts"`
	// DurationMS and TimedOutputTokens describe one measured generation
	// sample. Both are zero when the source exposes no request timing; a
	// non-zero duration must always be paired with the output it covers.
	DurationMS        int64     `json:"duration_ms,omitempty"`
	TimedOutputTokens int       `json:"timed_output_tokens,omitempty"`
	Timestamp         time.Time `json:"timestamp"`
	UUID              string    `json:"uuid"` // for dedup: Claude writes snapshots with same uuid
}

// TokenSummary is the lightweight aggregate advertised in LAN discovery packets.
type TokenSummary struct {
	Tokens24h   int                  `json:"tokens_24h"`
	TokensTotal int                  `json:"tokens_total"`
	CostTotal   float64              `json:"cost_total"`
	Sources     []TokenSourceSummary `json:"sources,omitempty"`
}

// TokenSourceSummary is a per-tool slice of a LAN token summary.
type TokenSourceSummary struct {
	Source      string  `json:"source"`
	Tokens24h   int     `json:"tokens_24h"`
	TokensTotal int     `json:"tokens_total"`
	CostTotal   float64 `json:"cost_total"`
}

// TrackPayload is the JSON payload for sending telemetry data to the remote server.
type TrackPayload struct {
	DeviceID string        `json:"device_id"`
	Type     string        `json:"type,omitempty"`      // "ping" or empty/"track"
	HTTPPort int           `json:"http_port,omitempty"` // for LAN auto-sync
	Summary  *TokenSummary `json:"summary,omitempty"`
	Usage    TokenUsage    `json:"usage"`
}

// SyncRecord represents a full database row for LAN auto-sync.
type SyncRecord struct {
	TokenUsage
	CostUSD    float64   `json:"cost_usd"`
	FilePath   string    `json:"file_path"`
	DeviceID   string    `json:"device_id"` // override usage's device ID
	Superseded bool      `json:"superseded,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// SyncPullResponse is a paginated LAN sync response.
type SyncPullResponse struct {
	Records       []SyncRecord `json:"records"`
	NextUpdatedAt time.Time    `json:"next_updated_at,omitempty"`
	NextAfterID   int64        `json:"next_after_id,omitempty"`
	HasMore       bool         `json:"has_more"`
}
