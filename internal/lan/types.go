package lan

import (
	"sync"
	"time"

	"token-ray/internal/model"
)

type PeerInfo struct {
	ID              string
	IP              string
	HTTPPort        int
	LastSeen        time.Time
	LastSync        time.Time
	LastSyncAttempt time.Time
	SyncStatus      string
	SyncError       string
	Summary         model.TokenSummary
	HasSummary      bool
}

type syncCursor struct {
	updatedAt time.Time
	afterID   int64
}

// LAN manages LAN-based device discovery and usage broadcasting.
// It replaces the former package-level global state with a testable struct.
type LAN struct {
	DeviceID string
	HTTPPort int

	mu              sync.RWMutex
	activePeers     map[string]PeerInfo
	peerUpdates     chan struct{}
	lastDirtySent   time.Time
	summarySource   func() model.TokenSummary
	summaryVersion  uint64
	summaryCacheTTL time.Duration
	cachedSummary   model.TokenSummary
	cachedSummaryAt time.Time
	cachedSummaryOK bool
	httpProbePorts  []int

	extraHostsMu       sync.RWMutex
	extraStaticHosts   []string
	tailscaleDiscovery bool
}

// New creates a new LAN instance for the given device.
func New(deviceID string, httpPort int) *LAN {
	return &LAN{
		DeviceID:        deviceID,
		HTTPPort:        httpPort,
		activePeers:     make(map[string]PeerInfo),
		peerUpdates:     make(chan struct{}, 1),
		summaryCacheTTL: SummaryCacheTTL,
		httpProbePorts:  normalizeHTTPDiscoveryPorts([]int{httpPort}),
	}
}
