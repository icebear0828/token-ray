package lan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"token-ray/internal/db"
	"token-ray/internal/model"
)

// StartAutoSync runs a background loop to pull DB records from active LAN peers via HTTP.
func (l *LAN) StartAutoSync(database *db.DB, token string) {
	l.StartAutoSyncContext(context.Background(), database, token)
}

// StartAutoSyncContext runs a background loop to pull DB records until ctx ends.
func (l *LAN) StartAutoSyncContext(ctx context.Context, database *db.DB, token string) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	lastSync := make(map[string]syncCursor)

	syncOnce := func() {
		l.mu.RLock()
		peers := make(map[string]PeerInfo)
		for id, peer := range l.activePeers {
			if id != l.DeviceID {
				peers[id] = peer
			}
		}
		l.mu.RUnlock()

		for id, peer := range peers {
			if peer.IP == "" || peer.HTTPPort == 0 {
				l.updatePeerSyncResult(id, "discovery_only", "", time.Time{})
				continue
			}

			l.syncWithPeer(id, peer, database, token, lastSync)
		}
	}

	syncOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		case <-l.peerUpdates:
			syncOnce()
		}
	}
}

func (l *LAN) syncWithPeer(id string, peer PeerInfo, database *db.DB, token string, lastSync map[string]syncCursor) {
	l.updatePeerSyncAttempt(id)

	peerCursorKey := id + "\x00peer"
	if !l.syncWithPeerScope(id, peer, database, token, lastSync, peerCursorKey, id) {
		return
	}
	if !l.syncWithPeerScope(id, peer, database, token, lastSync, id, "") {
		return
	}

	l.updatePeerSyncResult(id, "ok", "", time.Now().UTC())
}

func (l *LAN) syncWithPeerScope(id string, peer PeerInfo, database *db.DB, token string, lastSync map[string]syncCursor, cursorKey string, deviceID string) bool {
	baseURL := "http://" + net.JoinHostPort(peer.IP, strconv.Itoa(peer.HTTPPort)) + "/api/sync/pull"
	cursor := lastSync[cursorKey]

	client := &http.Client{Timeout: 10 * time.Second}
	for {
		values := url.Values{}
		values.Set("limit", "1000")
		if deviceID != "" {
			values.Set("device_id", deviceID)
		}
		if !cursor.updatedAt.IsZero() {
			values.Set("since", cursor.updatedAt.Format(time.RFC3339Nano))
		}
		if cursor.afterID > 0 {
			values.Set("after_id", strconv.FormatInt(cursor.afterID, 10))
		}

		req, err := http.NewRequest("GET", baseURL+"?"+values.Encode(), nil)
		if err != nil {
			l.updatePeerSyncResult(id, "error", err.Error(), time.Time{})
			return false
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			l.updatePeerSyncResult(id, "unreachable", err.Error(), time.Time{})
			return false
		}

		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			l.updatePeerSyncResult(id, "unauthorized", "remote rejected sync token", time.Time{})
			return false
		}
		if resp.StatusCode != http.StatusOK {
			status := resp.StatusCode
			resp.Body.Close()
			l.updatePeerSyncResult(id, "http_error", fmt.Sprintf("remote returned HTTP %d", status), time.Time{})
			return false
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			l.updatePeerSyncResult(id, "error", err.Error(), time.Time{})
			return false
		}

		var page model.SyncPullResponse
		if err := json.Unmarshal(body, &page); err != nil {
			var legacyRecords []model.SyncRecord
			if legacyErr := json.Unmarshal(body, &legacyRecords); legacyErr != nil {
				l.updatePeerSyncResult(id, "error", err.Error(), time.Time{})
				return false
			}
			page.Records = legacyRecords
			page.HasMore = false
		}

		for _, r := range page.Records {
			if err := database.UpsertSyncRecord(r); err != nil {
				errMsg := fmt.Sprintf("DB insert error for device %s: %v", r.DeviceID, err)
				log.Printf("LAN sync %s", errMsg)
				l.updatePeerSyncResult(id, "error", errMsg, time.Time{})
				return false
			}
		}
		if !page.NextUpdatedAt.IsZero() {
			cursor.updatedAt = page.NextUpdatedAt
			cursor.afterID = page.NextAfterID
		}
		if !page.HasMore {
			break
		}
		if page.NextUpdatedAt.IsZero() || page.NextAfterID == 0 {
			l.updatePeerSyncResult(id, "error", "remote returned incomplete sync cursor", time.Time{})
			return false
		}
	}

	if cursor.updatedAt.IsZero() {
		cursor.updatedAt = time.Now().UTC().Add(-5 * time.Minute)
	}
	lastSync[cursorKey] = cursor
	return true
}
