//go:build e2e

// E2E test for LAN multicast ping/discovery.
// Run with: go test -tags=e2e -v ./test/e2e/
package e2e

import (
	"context"
	"embed"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"token-ray/internal/lan"
	"token-ray/internal/model"
	"token-ray/internal/testutil"
	"token-ray/internal/web"
)

var emptyFS embed.FS

func TestLANPingDiscovery(t *testing.T) {
	t.Log("Starting E2E LAN Validation...")

	outChan := make(chan model.TokenUsage, 10)

	listener := lan.New("local-test-device", 19100)
	sender := lan.New("device-10-5", 19100)

	// Start listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.StartListenerContext(ctx, outChan)
	time.Sleep(1 * time.Second) // wait for listen

	// Try sending ping using current implementation
	sender.Ping()
	t.Log("Ping sent.")

	// Check if activePeers got it
	time.Sleep(1 * time.Second)
	peers := listener.GetActivePeers()
	t.Logf("Active peers detected: %v", peers)

	if containsPeer(peers, "device-10-5") {
		t.Log("E2E Test Passed! Listener received the ping.")
	} else {
		t.Errorf("E2E Test Failed! Expected peer 'device-10-5', got: %v", peers)
	}
}

func containsPeer(peers []string, target string) bool {
	for _, peer := range peers {
		if peer == target {
			return true
		}
	}
	return false
}

func TestLANBroadcastSummaryAppearsInScan(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	outChan := make(chan model.TokenUsage, 10)
	listener := lan.New("local-e2e-scan", 0)
	sender := lan.New("remote-e2e-broadcast", 0)
	sender.SetSummaryProvider(func() model.TokenSummary {
		return model.TokenSummary{
			Tokens24h:   4321,
			TokensTotal: 9876,
			CostTotal:   2.75,
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.StartListenerContext(ctx, outChan)

	handler := web.NewHandler(database, calc, nil, listener, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	sender.Ping()

	var data model.LANScanResponse
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(srv.URL + "/api/lan/scan")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if len(data.PeerInfos) == 1 && data.PeerInfos[0].TokensTotal == 9876 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(data.PeerInfos) != 1 {
		t.Fatalf("expected one peer from real LAN broadcast, got %+v", data)
	}

	peer := data.PeerInfos[0]
	if peer.ID != "remote-e2e-broadcast" || peer.Tokens24h != 4321 || peer.TokensTotal != 9876 || peer.CostTotal != 2.75 {
		t.Fatalf("expected broadcast token summary in scan response, got %+v", peer)
	}

	for i := 0; i < 3; i++ {
		resp, err := http.Get(srv.URL + "/api/lan/scan")
		if err != nil {
			t.Fatal(err)
		}
		var scan model.LANScanResponse
		if err := json.NewDecoder(resp.Body).Decode(&scan); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if len(scan.PeerInfos) != 1 || scan.PeerInfos[0].TokensTotal != 9876 {
			t.Fatalf("scan call %d: expected stable token summary, got %+v", i+1, scan)
		}
	}
}

func TestLegacyLANTrackPacketDoesNotForwardUsage(t *testing.T) {
	outChan := make(chan model.TokenUsage, 10)
	listener := lan.New("local-e2e-legacy", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.StartListenerContext(ctx, outChan)
	time.Sleep(1 * time.Second)

	sendLegacyTrackPacket(t, model.TrackPayload{
		DeviceID: "remote-legacy-broadcast",
		Type:     "track",
		Usage: model.TokenUsage{
			Source:       "Claude Code",
			Model:        "claude-opus-4-7",
			InputTokens:  1234,
			OutputTokens: 56,
			UUID:         "legacy-lan-track-1",
			Timestamp:    time.Now().UTC(),
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	seenPeer := false
	for time.Now().Before(deadline) {
		if containsPeer(listener.GetActivePeers(), "remote-legacy-broadcast") {
			seenPeer = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !seenPeer {
		t.Fatal("expected legacy UDP track packet to still record peer discovery")
	}

	select {
	case usage := <-outChan:
		t.Fatalf("expected unauthenticated UDP track usage to be ignored, got %+v", usage)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestActiveHTTPDiscoveryAppearsInScan(t *testing.T) {
	remoteDB, remoteCalc := testutil.NewTestDBAndCalc(t)
	defer remoteDB.Close()

	remoteLAN := lan.New("remote-e2e-http", 0)
	remoteLAN.SetSummaryProvider(func() model.TokenSummary {
		return model.TokenSummary{
			Tokens24h:   1357,
			TokensTotal: 2468,
			CostTotal:   3.14,
		}
	})
	remoteSrv := httptest.NewServer(web.NewHandler(remoteDB, remoteCalc, nil, remoteLAN, "", emptyFS))
	defer remoteSrv.Close()
	host, port := splitServerURL(t, remoteSrv.URL)

	localDB, localCalc := testutil.NewTestDBAndCalc(t)
	defer localDB.Close()
	localLAN := lan.New("local-e2e-http", 0)
	localLAN.ScanHTTPPeers(context.Background(), []string{host}, []int{port})

	localSrv := httptest.NewServer(web.NewHandler(localDB, localCalc, nil, localLAN, "", emptyFS))
	defer localSrv.Close()

	for i := 0; i < 3; i++ {
		resp, err := http.Get(localSrv.URL + "/api/lan/scan")
		if err != nil {
			t.Fatal(err)
		}
		var scan model.LANScanResponse
		if err := json.NewDecoder(resp.Body).Decode(&scan); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if len(scan.PeerInfos) != 1 {
			t.Fatalf("scan call %d: expected active HTTP peer, got %+v", i+1, scan)
		}
		peer := scan.PeerInfos[0]
		if peer.ID != "remote-e2e-http" || peer.SyncStatus != "discovery_only" || peer.Tokens24h != 1357 || peer.TokensTotal != 2468 || peer.CostTotal != 3.14 {
			t.Fatalf("scan call %d: expected active HTTP summary, got %+v", i+1, peer)
		}
	}
}

func TestZeroConfigHTTPDiscoveryPullsRecords(t *testing.T) {
	remoteDB, remoteCalc := testutil.NewTestDBAndCalc(t)
	defer remoteDB.Close()

	now := time.Now().UTC()
	if err := remoteDB.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Claude Code",
			Model:        "claude-opus-4-7",
			InputTokens:  1000,
			OutputTokens: 250,
			UUID:         "zero-config-e2e",
			Timestamp:    now,
		},
		1.50,
		now,
		"/remote/session.jsonl",
		"remote-zero-config",
	); err != nil {
		t.Fatal(err)
	}

	remoteLAN := lan.New("remote-zero-config", 0)
	remoteSrv := httptest.NewServer(web.NewHandler(remoteDB, remoteCalc, nil, remoteLAN, "", emptyFS))
	defer remoteSrv.Close()
	host, port := splitServerURL(t, remoteSrv.URL)
	remoteLAN.HTTPPort = port

	localDB, _ := testutil.NewTestDBAndCalc(t)
	defer localDB.Close()
	localLAN := lan.New("local-zero-config", 19100)
	localLAN.ScanHTTPPeers(context.Background(), []string{host}, []int{port})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go localLAN.StartAutoSyncContext(ctx, localDB, "")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, input, _, _, output, err := localDB.QueryPeriodStatsAll("remote-zero-config", "")
		if err != nil {
			t.Fatal(err)
		}
		if input == 1000 && output == 250 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	_, input, _, _, output, err := localDB.QueryPeriodStatsAll("remote-zero-config", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("expected zero-config LAN sync to pull remote records, got input=%d output=%d", input, output)
}

func TestZeroConfigHTTPDiscoveryPrioritizesPeerDeviceRecords(t *testing.T) {
	remoteDB, remoteCalc := testutil.NewTestDBAndCalc(t)
	defer remoteDB.Close()

	now := time.Now().UTC()
	if err := remoteDB.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, UUID: "local-backfill-e2e", Timestamp: now.Add(-time.Hour)},
		1.00,
		now.Add(-time.Hour),
		"/local/session.jsonl",
		"local-zero-config-priority",
	); err != nil {
		t.Fatal(err)
	}
	if err := remoteDB.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, OutputTokens: 500, UUID: "remote-priority-e2e", Timestamp: now},
		2.00,
		now,
		"/remote/session.jsonl",
		"remote-zero-config-priority",
	); err != nil {
		t.Fatal(err)
	}

	remoteLAN := lan.New("remote-zero-config-priority", 0)
	remoteHandler := web.NewHandler(remoteDB, remoteCalc, nil, remoteLAN, "", emptyFS)
	var syncQueries []string
	remoteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync/pull" {
			syncQueries = append(syncQueries, r.URL.RawQuery)
		}
		remoteHandler.ServeHTTP(w, r)
	}))
	defer remoteSrv.Close()
	host, port := splitServerURL(t, remoteSrv.URL)
	remoteLAN.HTTPPort = port

	localDB, _ := testutil.NewTestDBAndCalc(t)
	defer localDB.Close()
	localLAN := lan.New("local-zero-config-priority", 19100)
	localLAN.ScanHTTPPeers(context.Background(), []string{host}, []int{port})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go localLAN.StartAutoSyncContext(ctx, localDB, "")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, input, _, _, output, err := localDB.QueryPeriodStatsAll("remote-zero-config-priority", "")
		if err != nil {
			t.Fatal(err)
		}
		if input == 2000 && output == 500 {
			if len(syncQueries) == 0 || !strings.Contains(syncQueries[0], "device_id=remote-zero-config-priority") {
				t.Fatalf("expected first sync query to prioritize peer device, got %+v", syncQueries)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("expected prioritized zero-config sync to pull peer records, queries=%+v", syncQueries)
}

func splitServerURL(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func sendLegacyTrackPacket(t *testing.T, payload model.TrackPayload) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := net.ResolveUDPAddr("udp", lan.MulticastAddr)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(data); err != nil {
		t.Fatal(err)
	}
}
