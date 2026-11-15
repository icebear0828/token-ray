package lan

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"token-ray/internal/model"
	"token-ray/internal/testutil"
)

func testPeerFromServerURL(t *testing.T, rawURL string) PeerInfo {
	t.Helper()
	host, port := testHostPortFromServerURL(t, rawURL)
	return PeerInfo{ID: "remote", IP: host, HTTPPort: port, LastSeen: time.Now()}
}

func testHostPortFromServerURL(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(rawURL[len("http://"):])
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestScanHTTPPeersRecordsSelfEndpointSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/lan/self" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.LANSelfResponse{
			DeviceID: "remote-http",
			HTTPPort: 19100,
			Summary: &model.TokenSummary{
				Tokens24h:   1200,
				TokensTotal: 3400,
				CostTotal:   1.23,
			},
		})
	}))
	defer server.Close()

	host, port := testHostPortFromServerURL(t, server.URL)
	l := New("local", 0)
	l.ScanHTTPPeers(context.Background(), []string{host}, []int{port})

	peers := l.GetActivePeerInfos()
	if len(peers) != 1 {
		t.Fatalf("expected one actively discovered peer, got %+v", peers)
	}
	peer := peers[0]
	if peer.ID != "remote-http" || peer.IP != host || peer.HTTPPort != 19100 {
		t.Fatalf("unexpected active peer identity: %+v", peer)
	}
	if !peer.HasSummary || peer.Summary.TokensTotal != 3400 || peer.Summary.Tokens24h != 1200 || peer.Summary.CostTotal != 1.23 {
		t.Fatalf("expected active peer summary, got %+v", peer)
	}
}

func TestScanHTTPPeersAllowsModeratelySlowLANSelfEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/lan/self" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(400 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.LANSelfResponse{
			DeviceID: "remote-slow",
			HTTPPort: 19100,
			Summary: &model.TokenSummary{
				TokensTotal: 1234,
			},
		})
	}))
	defer server.Close()

	host, port := testHostPortFromServerURL(t, server.URL)
	l := New("local", 0)
	l.ScanHTTPPeers(context.Background(), []string{host}, []int{port})

	peers := l.GetActivePeerInfos()
	if len(peers) != 1 {
		t.Fatalf("expected moderately slow LAN peer to be discovered, got %+v", peers)
	}
	if peers[0].ID != "remote-slow" || !peers[0].HasSummary || peers[0].Summary.TokensTotal != 1234 {
		t.Fatalf("unexpected slow peer info: %+v", peers[0])
	}
}

func TestScanHTTPPeersSkipsLocalDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/lan/self" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.LANSelfResponse{DeviceID: "local", HTTPPort: 19100})
	}))
	defer server.Close()

	host, port := testHostPortFromServerURL(t, server.URL)
	l := New("local", 0)
	l.ScanHTTPPeers(context.Background(), []string{host}, []int{port})

	if peers := l.GetActivePeerInfos(); len(peers) != 0 {
		t.Fatalf("expected local device to be ignored, got %+v", peers)
	}
}

func TestProbeHostsForIPv4UsesLocalSubnet(t *testing.T) {
	hosts := probeHostsForIPv4(net.ParseIP("192.168.10.6"), net.CIDRMask(30, 32))
	if len(hosts) != 1 || hosts[0] != "192.168.10.5" {
		t.Fatalf("expected /30 peer host only, got %+v", hosts)
	}

	hosts = probeHostsForIPv4(net.ParseIP("192.168.10.6"), net.CIDRMask(16, 32))
	if len(hosts) != 253 {
		t.Fatalf("expected broad subnet to be capped to local /24, got %d hosts", len(hosts))
	}
	if !hasHost(hosts, "192.168.10.5") || hasHost(hosts, "192.168.11.1") || hasHost(hosts, "192.168.10.6") {
		t.Fatalf("expected local /24 peers excluding self, got %+v", hosts)
	}
}

func hasHost(hosts []string, target string) bool {
	for _, host := range hosts {
		if host == target {
			return true
		}
	}
	return false
}

func TestSyncWithPeerUpdatesDatabaseAndStatus(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	records := []model.SyncRecord{
		{
			TokenUsage: model.TokenUsage{
				Source:       "Claude Code",
				Model:        "claude-opus-4-7",
				InputTokens:  1000,
				OutputTokens: 200,
				Timestamp:    now,
				UUID:         "remote-uuid-1",
			},
			CostUSD:   1.25,
			FilePath:  "/remote/session.jsonl",
			DeviceID:  "remote",
			UpdatedAt: now,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sync/pull" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SyncPullResponse{Records: records, NextUpdatedAt: now, NextAfterID: 1})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 1000 || output != 200 {
		t.Fatalf("expected synced remote tokens, got input=%d output=%d", input, output)
	}

	l.mu.RLock()
	status := l.activePeers["remote"].SyncStatus
	errMsg := l.activePeers["remote"].SyncError
	l.mu.RUnlock()
	if status != "ok" || errMsg != "" {
		t.Fatalf("expected ok sync status, got status=%q error=%q", status, errMsg)
	}
}

func TestSyncWithPeerPrioritizesPeerDeviceRecords(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sync/pull" {
			http.NotFound(w, r)
			return
		}
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("device_id") == "remote" {
			_ = json.NewEncoder(w).Encode(model.SyncPullResponse{
				Records: []model.SyncRecord{{
					TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 900, OutputTokens: 90, Timestamp: now, UUID: "remote-priority"},
					CostUSD:    9.00,
					DeviceID:   "remote",
					UpdatedAt:  now,
				}},
				NextUpdatedAt: now,
				NextAfterID:   1,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(model.SyncPullResponse{
			Records: []model.SyncRecord{{
				TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, Timestamp: now.Add(-time.Hour), UUID: "local-backfill"},
				CostUSD:    1.00,
				DeviceID:   "local",
				UpdatedAt:  now.Add(-time.Hour),
			}},
			NextUpdatedAt: now.Add(-time.Hour),
			NextAfterID:   2,
		})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	peer.ID = "remote"
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	if len(queries) < 2 || !strings.Contains(queries[0], "device_id=remote") {
		t.Fatalf("expected first sync request to prioritize peer device, got queries=%+v", queries)
	}
	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 900 || output != 90 {
		t.Fatalf("expected prioritized peer records to sync, got input=%d output=%d", input, output)
	}
}

func TestSyncWithPeerRecordsUnauthorizedStatus(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "wrong-token", map[string]syncCursor{})

	l.mu.RLock()
	status := l.activePeers["remote"].SyncStatus
	errMsg := l.activePeers["remote"].SyncError
	l.mu.RUnlock()
	if status != "unauthorized" || errMsg == "" {
		t.Fatalf("expected unauthorized sync status, got status=%q error=%q", status, errMsg)
	}
}

func TestRecordPeerWithoutHTTPPortIsDiscoveryOnly(t *testing.T) {
	l := New("local", 19100)
	l.RecordPeer("remote", "192.168.1.25", 0)

	peers := l.GetActivePeerInfos()
	if len(peers) != 1 {
		t.Fatalf("expected one peer, got %+v", peers)
	}
	if peers[0].SyncStatus != "discovery_only" || peers[0].SyncError != "" {
		t.Fatalf("expected discovery-only peer, got %+v", peers[0])
	}

	l.RecordPeer("remote", "192.168.1.25", 19100)
	peers = l.GetActivePeerInfos()
	if len(peers) != 1 {
		t.Fatalf("expected one peer after update, got %+v", peers)
	}
	if peers[0].SyncStatus != "pending" {
		t.Fatalf("expected peer to return to pending when sync port appears, got %+v", peers[0])
	}
}

func TestSyncWithPeerFollowsPagination(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_ = json.NewEncoder(w).Encode(model.SyncPullResponse{
				Records: []model.SyncRecord{{
					TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, Timestamp: now, UUID: "page-1"},
					CostUSD:    1.00,
					DeviceID:   "remote",
					UpdatedAt:  now,
				}},
				NextUpdatedAt: now,
				NextAfterID:   1,
				HasMore:       true,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(model.SyncPullResponse{
			Records: []model.SyncRecord{{
				TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, Timestamp: now.Add(time.Second), UUID: "page-2"},
				CostUSD:    2.00,
				DeviceID:   "remote",
				UpdatedAt:  now.Add(time.Second),
			}},
		})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	if calls != 4 {
		t.Fatalf("expected paginated priority and full sync calls, got %d", calls)
	}
	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 300 || output != 30 {
		t.Fatalf("expected both pages to sync, got input=%d output=%d", input, output)
	}
}

func TestSyncWithPeerAcceptsLegacyArrayResponse(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]model.SyncRecord{{
			TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 400, OutputTokens: 40, Timestamp: now, UUID: "legacy-array-1"},
			CostUSD:    4.00,
			DeviceID:   "remote",
			UpdatedAt:  now,
		}})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 400 || output != 40 {
		t.Fatalf("expected legacy array response to sync, got input=%d output=%d", input, output)
	}
}
