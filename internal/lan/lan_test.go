package lan_test

import (
	"testing"
	"time"

	"token-ray/internal/lan"
	"token-ray/internal/model"
)

func TestNew(t *testing.T) {
	l := lan.New("test-device", 19100)
	if l.DeviceID != "test-device" {
		t.Errorf("expected device ID 'test-device', got %q", l.DeviceID)
	}
}

func TestCurrentSummaryCachesProvider(t *testing.T) {
	l := lan.New("local", 19100)
	calls := 0
	l.SetSummaryProvider(func() model.TokenSummary {
		calls++
		return model.TokenSummary{TokensTotal: calls}
	})

	first, ok := l.CurrentSummary()
	if !ok {
		t.Fatal("expected summary provider to be available")
	}
	second, ok := l.CurrentSummary()
	if !ok {
		t.Fatal("expected cached summary to be available")
	}

	if calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", calls)
	}
	if first.TokensTotal != 1 || second.TokensTotal != 1 {
		t.Fatalf("expected cached summary value, got first=%+v second=%+v", first, second)
	}
}

func TestGetActivePeers_Empty(t *testing.T) {
	l := lan.New("local", 19100)
	peers := l.GetActivePeers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestGetActivePeers_ExcludesSelf(t *testing.T) {
	l := lan.New("local", 19100)
	l.RecordPeer("local", "127.0.0.1", 19100)  // self
	l.RecordPeer("remote", "127.0.0.2", 19100) // other device

	peers := l.GetActivePeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer (excluding self), got %d", len(peers))
	}
	if peers[0] != "remote" {
		t.Errorf("expected 'remote', got %q", peers[0])
	}
}

func TestGetActivePeers_EvictsStale(t *testing.T) {
	l := lan.New("local", 19100)

	// Record a peer as stale (older than PeerTTL)
	l.RecordPeerAt("stale-device", "127.0.0.2", 19100, time.Now().Add(-lan.PeerTTL-1*time.Second))
	// Record a fresh peer
	l.RecordPeer("fresh-device", "127.0.0.3", 19100)

	peers := l.GetActivePeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer (stale evicted), got %d: %v", len(peers), peers)
	}
	if peers[0] != "fresh-device" {
		t.Errorf("expected 'fresh-device', got %q", peers[0])
	}
}

func TestGetActivePeers_MultiplePeers(t *testing.T) {
	l := lan.New("local", 19100)

	l.RecordPeer("device-a", "127.0.0.1", 19100)
	l.RecordPeer("device-b", "127.0.0.2", 19100)
	l.RecordPeer("device-c", "127.0.0.3", 19100)

	peers := l.GetActivePeers()
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers, got %d: %v", len(peers), peers)
	}

	// Verify all are present (order is map-iteration-dependent)
	found := make(map[string]bool)
	for _, p := range peers {
		found[p] = true
	}
	for _, expected := range []string{"device-a", "device-b", "device-c"} {
		if !found[expected] {
			t.Errorf("missing expected peer %q", expected)
		}
	}
}

func TestRecordPeer_UpdatesTimestamp(t *testing.T) {
	l := lan.New("local", 19100)

	// Record peer initially as stale
	l.RecordPeerAt("device-x", "127.0.0.1", 19100, time.Now().Add(-lan.PeerTTL-1*time.Second))

	// Should be evicted now
	peers := l.GetActivePeers()
	if len(peers) != 0 {
		t.Fatalf("stale peer should be evicted, got %v", peers)
	}

	// Re-record with fresh timestamp
	l.RecordPeer("device-x", "127.0.0.1", 19100)

	peers = l.GetActivePeers()
	if len(peers) != 1 {
		t.Fatalf("re-recorded peer should be active, got %d peers", len(peers))
	}
}
