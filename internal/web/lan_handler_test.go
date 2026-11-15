package web_test

import (
	"token-ray/internal/lan"
	"token-ray/internal/model"
	"token-ray/internal/testutil"
	"token-ray/internal/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLANScanIncludesPeerInfoAndTokenSummary(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	for _, row := range []struct {
		usage model.TokenUsage
		cost  float64
		path  string
	}{
		{
			usage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
			cost:  1.50,
			path:  "/remote-claude.jsonl",
		},
		{
			usage: model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 3000, OutputTokens: 400},
			cost:  3.40,
			path:  "/remote-codex.jsonl",
		},
	} {
		if err := database.InsertUsageWithTime(row.usage, row.cost, now.Add(-10*time.Minute), row.path, "remote-device"); err != nil {
			t.Fatal(err)
		}
	}

	lanInst := lan.New("local-device", 19100)
	lanInst.RecordPeer("remote-device", "192.168.1.25", 19100)

	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/scan")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.LANScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Peers) != 1 || data.Peers[0] != "remote-device" {
		t.Fatalf("expected compatible peers list, got %+v", data.Peers)
	}
	if len(data.PeerInfos) != 1 {
		t.Fatalf("expected one peer info, got %+v", data.PeerInfos)
	}
	peer := data.PeerInfos[0]
	if peer.ID != "remote-device" || peer.IP != "192.168.1.25" || peer.HTTPPort != 19100 {
		t.Fatalf("unexpected peer info: %+v", peer)
	}
	if peer.SyncStatus != "pending" {
		t.Fatalf("expected pending sync status, got %q", peer.SyncStatus)
	}
	if peer.Tokens24h != 4600 || peer.TokensTotal != 4600 {
		t.Fatalf("expected token summaries to include synced DB data, got %+v", peer)
	}
	if len(peer.Sources) != 2 || peer.Sources[0].Source != "Claude Code" || peer.Sources[1].Source != "Codex" {
		t.Fatalf("expected LAN peer source summaries, got %+v", peer.Sources)
	}
}
func TestLANScanUsesAdvertisedSummaryWhenPeerIsDiscoveryOnly(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	lanInst.RecordPeerSummary("remote-device", "192.168.1.25", 0, model.TokenSummary{
		Tokens24h:   1200,
		TokensTotal: 3400,
		CostTotal:   1.23,
		Sources: []model.TokenSourceSummary{
			{Source: "Claude Code", Tokens24h: 700, TokensTotal: 2100, CostTotal: 0.80},
			{Source: "Gemini CLI", Tokens24h: 500, TokensTotal: 1300, CostTotal: 0.43},
		},
	})

	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/scan")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.LANScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.PeerInfos) != 1 {
		t.Fatalf("expected one peer info, got %+v", data.PeerInfos)
	}
	peer := data.PeerInfos[0]
	if peer.SyncStatus != "discovery_only" {
		t.Fatalf("expected discovery_only sync status, got %+v", peer)
	}
	if peer.Tokens24h != 1200 || peer.TokensTotal != 3400 || peer.CostTotal != 1.23 {
		t.Fatalf("expected advertised token summary, got %+v", peer)
	}
	if len(peer.Sources) != 2 || peer.Sources[0].Source != "Claude Code" || peer.Sources[1].Source != "Gemini CLI" {
		t.Fatalf("expected advertised source summaries, got %+v", peer.Sources)
	}
}
func TestLANSelfEndpointIncludesDeviceAndSummary(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	lanInst.SetSummaryProvider(func() model.TokenSummary {
		return model.TokenSummary{
			Tokens24h:   222,
			TokensTotal: 333,
			CostTotal:   4.56,
			Sources: []model.TokenSourceSummary{
				{Source: "Codex", Tokens24h: 222, TokensTotal: 333, CostTotal: 4.56},
			},
		}
	})

	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/self")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.LANSelfResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.DeviceID != "local-device" || data.HTTPPort != 19100 {
		t.Fatalf("unexpected LAN self identity: %+v", data)
	}
	if data.Summary == nil || data.Summary.Tokens24h != 222 || data.Summary.TokensTotal != 333 || data.Summary.CostTotal != 4.56 {
		t.Fatalf("unexpected LAN self summary: %+v", data.Summary)
	}
	if len(data.Summary.Sources) != 1 || data.Summary.Sources[0].Source != "Codex" {
		t.Fatalf("unexpected LAN self source summary: %+v", data.Summary.Sources)
	}
}
func TestLANScanAndJoinRemainAvailableWithSyncToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	lanInst.RecordPeer("remote-device", "192.168.1.25", 0)

	handler := web.NewHandler(database, calc, nil, lanInst, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/scan")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected unauthenticated UI LAN scan to remain available with sync token, got %d", resp.StatusCode)
	}

	var data model.LANScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.PeerInfos) != 1 || data.PeerInfos[0].ID != "remote-device" {
		t.Fatalf("expected LAN peer info, got %+v", data)
	}

	resp, err = http.Post(srv.URL+"/api/lan/join", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected unauthenticated UI LAN join to remain available with sync token, got %d", resp.StatusCode)
	}
}
func TestLANStatusJoinAndLeave(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(srv.URL + "/api/lan/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected LAN status 200, got %d", resp.StatusCode)
	}
	var status model.LANStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled {
		t.Fatalf("expected LAN enabled before leave, got %+v", status)
	}

	resp, err = client.Post(srv.URL+"/api/lan/leave", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected LAN leave 200, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatalf("expected LAN disabled after leave, got %+v", status)
	}

	resp, err = client.Post(srv.URL+"/api/lan/join", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected LAN join 200, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled {
		t.Fatalf("expected LAN enabled after join, got %+v", status)
	}
}
