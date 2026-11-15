package web_test

import (
	"token-ray/internal/lan"
	"token-ray/internal/model"
	"token-ray/internal/testutil"
	"token-ray/internal/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestSyncPullPaginates(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	for i, uuid := range []string{"sync-pull-1", "sync-pull-2"} {
		if err := database.InsertUsageWithTime(
			model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100 + i, OutputTokens: 10, UUID: uuid},
			1.00, now.Add(time.Duration(i)*time.Second), "/sync.jsonl", "remote",
		); err != nil {
			t.Fatal(err)
		}
	}

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sync/pull?limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var first model.SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || !first.HasMore || first.NextUpdatedAt.IsZero() || first.NextAfterID == 0 {
		t.Fatalf("unexpected first sync page: %+v", first)
	}

	nextURL := srv.URL + "/api/sync/pull?limit=1&since=" + first.NextUpdatedAt.Format(time.RFC3339Nano) + "&after_id=" + strconv.FormatInt(first.NextAfterID, 10)
	req, _ = http.NewRequest(http.MethodGet, nextURL, nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var second model.SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].UUID == first.Records[0].UUID {
		t.Fatalf("expected cursor to advance, first=%+v second=%+v", first, second)
	}
}
func TestSyncPullFiltersDeviceID(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, UUID: "local-first"},
		1.00, now, "/local.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, UUID: "remote-first"},
		2.00, now.Add(time.Second), "/remote.jsonl", "remote",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sync/pull?limit=1&device_id=remote", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected sync pull OK, got %d", resp.StatusCode)
	}

	var page model.SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].DeviceID != "remote" || page.Records[0].UUID != "remote-first" {
		t.Fatalf("expected filtered remote sync record, got %+v", page)
	}
}
func TestLANHandlerExposesOnlySyncSurface(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	handler := web.NewLANHandler(database, "secret-token", lanInst)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected minimal LAN handler to hide dashboard API, got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sync/pull?limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected authorized sync pull, got %d", resp.StatusCode)
	}
}
func TestSyncPullAllowsPrivateLANWithoutToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200, UUID: "zero-config-sync"},
		1.50, now, "/remote.jsonl", "local-device",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/pull", nil)
	req.RemoteAddr = "192.168.10.5:42310"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected private LAN sync pull without token, got %d", rec.Code)
	}

	var data model.SyncPullResponse
	if err := json.NewDecoder(rec.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Records) != 1 || data.Records[0].UUID != "zero-config-sync" {
		t.Fatalf("expected zero-config sync record, got %+v", data)
	}
}
func TestSyncPullRejectsPublicRemoteWithoutToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/pull", nil)
	req.RemoteAddr = "203.0.113.5:42310"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected public sync pull without token to be rejected, got %d", rec.Code)
	}
}
