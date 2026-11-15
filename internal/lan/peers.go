package lan

import (
	"sort"
	"time"

	"token-ray/internal/model"
)

// GetActivePeers returns a list of recently seen remote device IDs.
// Excludes the local device and evicts stale entries.
func (l *LAN) GetActivePeers() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var peers []string
	now := time.Now()
	for id, peer := range l.activePeers {
		if now.Sub(peer.LastSeen) >= PeerTTL {
			delete(l.activePeers, id)
			continue
		}
		if id != l.DeviceID {
			peers = append(peers, id)
		}
	}
	return peers
}

// GetActivePeerInfos returns recently seen remote peers with sync status.
func (l *LAN) GetActivePeerInfos() []PeerInfo {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	peers := make([]PeerInfo, 0, len(l.activePeers))
	for id, peer := range l.activePeers {
		if now.Sub(peer.LastSeen) >= PeerTTL {
			delete(l.activePeers, id)
			continue
		}
		if id == l.DeviceID {
			continue
		}
		peer.ID = id
		peers = append(peers, peer)
	}
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].ID < peers[j].ID
	})
	return peers
}

// SetSummaryProvider sets the aggregate token summary advertised in LAN packets.
func (l *LAN) SetSummaryProvider(provider func() model.TokenSummary) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.summarySource = provider
	l.summaryVersion++
	l.cachedSummaryOK = false
}

// CurrentSummary returns the current aggregate token summary, if configured.
func (l *LAN) CurrentSummary() (model.TokenSummary, bool) {
	now := time.Now()
	l.mu.RLock()
	provider := l.summarySource
	version := l.summaryVersion
	ttl := l.summaryCacheTTL
	if ttl <= 0 {
		ttl = SummaryCacheTTL
	}
	if l.cachedSummaryOK && now.Sub(l.cachedSummaryAt) < ttl {
		summary := l.cachedSummary
		l.mu.RUnlock()
		return summary, true
	}
	l.mu.RUnlock()
	if provider == nil {
		return model.TokenSummary{}, false
	}
	summary := provider()

	l.mu.Lock()
	if l.summaryVersion == version {
		l.cachedSummary = summary
		l.cachedSummaryAt = now
		l.cachedSummaryOK = true
	}
	l.mu.Unlock()
	return summary, true
}

// SetHTTPDiscoveryPorts configures local HTTP ports to probe for active peers.
func (l *LAN) SetHTTPDiscoveryPorts(ports ...int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.httpProbePorts = normalizeHTTPDiscoveryPorts(ports)
}

// SetExtraPeerHosts configures user-supplied hostnames/IPs that should be
// probed in addition to local subnet sweeps. Useful for Tailscale, VPN, or
// cross-subnet peers that broadcast/multicast cannot reach.
func (l *LAN) SetExtraPeerHosts(hosts []string) {
	cleaned := NormalizeExtraPeerHosts(hosts)
	l.extraHostsMu.Lock()
	defer l.extraHostsMu.Unlock()
	l.extraStaticHosts = cleaned
}

// SetTailscaleDiscovery toggles automatic peer discovery via the `tailscale`
// CLI. When enabled, online Tailscale peers are unicast-probed each scan.
func (l *LAN) SetTailscaleDiscovery(enabled bool) {
	l.extraHostsMu.Lock()
	defer l.extraHostsMu.Unlock()
	l.tailscaleDiscovery = enabled
}

func (l *LAN) extraHostsSnapshot() (static []string, tailscale bool) {
	l.extraHostsMu.RLock()
	defer l.extraHostsMu.RUnlock()
	return append([]string(nil), l.extraStaticHosts...), l.tailscaleDiscovery
}

func (l *LAN) httpDiscoveryPorts() []int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]int(nil), l.httpProbePorts...)
}

func normalizeHTTPDiscoveryPorts(ports []int) []int {
	seen := make(map[int]bool)
	normalized := make([]int, 0, len(ports)+1)
	for _, port := range ports {
		if port <= 0 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true
		normalized = append(normalized, port)
	}
	if len(normalized) == 0 {
		normalized = append(normalized, DefaultHTTPPort)
	}
	return normalized
}

// RecordPeer records a peer as active. Exposed for testing.
func (l *LAN) RecordPeer(deviceID, ip string, httpPort int) {
	l.recordPeer(deviceID, ip, httpPort, nil, true)
}

// RecordPeerSummary records a peer and its advertised token summary.
func (l *LAN) RecordPeerSummary(deviceID, ip string, httpPort int, summary model.TokenSummary) {
	l.recordPeer(deviceID, ip, httpPort, &summary, true)
}

func (l *LAN) recordPeer(deviceID, ip string, httpPort int, summary *model.TokenSummary, notify bool) {
	if deviceID == "" {
		return
	}
	l.mu.Lock()
	peer := l.activePeers[deviceID]
	wasNew := peer.ID == ""
	endpointChanged := peer.IP != ip || peer.HTTPPort != httpPort
	peer.ID = deviceID
	peer.IP = ip
	peer.HTTPPort = httpPort
	peer.LastSeen = time.Now()
	if httpPort == 0 {
		peer.SyncStatus = "discovery_only"
		peer.SyncError = ""
	} else if peer.SyncStatus == "discovery_only" {
		peer.SyncStatus = "pending"
	}
	if peer.SyncStatus == "" {
		peer.SyncStatus = "pending"
	}
	if summary != nil {
		peer.Summary = *summary
		peer.HasSummary = true
	}
	l.activePeers[deviceID] = peer
	l.mu.Unlock()

	if notify || wasNew || endpointChanged {
		select {
		case l.peerUpdates <- struct{}{}:
		default:
		}
	}
}

// RecordPeerAt records a peer with a specific timestamp. Exposed for testing.
func (l *LAN) RecordPeerAt(deviceID, ip string, httpPort int, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activePeers[deviceID] = PeerInfo{
		ID:         deviceID,
		IP:         ip,
		HTTPPort:   httpPort,
		LastSeen:   at,
		SyncStatus: "pending",
	}
}

func (l *LAN) newPayload(payloadType string) model.TrackPayload {
	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
		HTTPPort: l.HTTPPort,
		Type:     payloadType,
	}

	if summary, ok := l.CurrentSummary(); ok {
		payload.Summary = &summary
	}
	return payload
}

func (l *LAN) updatePeerSyncAttempt(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	peer := l.activePeers[id]
	peer.ID = id
	peer.LastSyncAttempt = time.Now()
	if peer.SyncStatus == "" || peer.SyncStatus == "pending" {
		peer.SyncStatus = "syncing"
	}
	l.activePeers[id] = peer
}

func (l *LAN) updatePeerSyncResult(id string, status string, errMsg string, syncedAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	peer := l.activePeers[id]
	peer.ID = id
	peer.SyncStatus = status
	peer.SyncError = errMsg
	if status == "ok" {
		peer.LastSync = syncedAt
	}
	l.activePeers[id] = peer
}
