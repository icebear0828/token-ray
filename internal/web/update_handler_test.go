package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/updater"
	"ai-flight-dashboard/internal/web"
)

func TestHandleSystemVersion(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)

	req := httptest.NewRequest(http.MethodGet, "/api/system/version", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res struct {
		Version string `json:"version"`
		GOOS    string `json:"goos"`
		GOARCH  string `json:"goarch"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Version == "" {
		t.Error("expected non-empty version")
	}
	if res.GOOS == "" || res.GOARCH == "" {
		t.Error("expected non-empty runtime info")
	}
}

func TestHandleUpdateCheck_HasUpdate(t *testing.T) {
	mockRelease := updater.Release{
		TagName: "v9.9.9",
		Name:    "v9.9.9",
		Body:    "Major release",
		Assets: []updater.Asset{
			{Name: "ai-flight-dashboard-macos-m-series.tar.gz", URL: "https://example.com/asset"},
		},
	}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockRelease)
	}))
	defer apiServer.Close()

	// Redirect default base; use a unique repo so the global check cache cannot leak in.
	origBase := updater.GitHubAPIBase
	origRepo := updater.GitHubRepo
	updater.GitHubAPIBase = apiServer.URL
	updater.GitHubRepo = "has-update/test"
	defer func() {
		updater.GitHubAPIBase = origBase
		updater.GitHubRepo = origRepo
	}()

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)

	req := httptest.NewRequest(http.MethodGet, "/api/system/update/check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var status updater.UpdateStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !status.HasUpdate {
		t.Errorf("expected has_update=true, got %v", status.HasUpdate)
	}
	if status.LatestVersion != "v9.9.9" {
		t.Errorf("expected latest_version=v9.9.9, got %s", status.LatestVersion)
	}
}

func TestHandleUpdateApply_UnauthorizedWhenTokenRequired(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	// Requires token "secret123"
	handler := web.NewHandler(database, calc, nil, nil, "secret123", emptyFS)

	req := httptest.NewRequest(http.MethodPost, "/api/system/update/apply", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestHandleUpdateCheck_ErrorIsFlat(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer apiServer.Close()

	origBase := updater.GitHubAPIBase
	origRepo := updater.GitHubRepo
	updater.GitHubAPIBase = apiServer.URL
	updater.GitHubRepo = "error/test"
	defer func() {
		updater.GitHubAPIBase = origBase
		updater.GitHubRepo = origRepo
	}()

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)

	req := httptest.NewRequest(http.MethodGet, "/api/system/update/check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var status updater.UpdateStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.HasUpdate {
		t.Error("expected has_update=false on error")
	}
	if status.Error == "" {
		t.Error("expected error message in the flat response")
	}
}

func TestHandleUpdateCheck_CachesResult(t *testing.T) {
	var hits int
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updater.Release{TagName: "v9.9.9"})
	}))
	defer apiServer.Close()

	origBase := updater.GitHubAPIBase
	origRepo := updater.GitHubRepo
	updater.GitHubAPIBase = apiServer.URL
	updater.GitHubRepo = "cache-test/repo"
	defer func() {
		updater.GitHubAPIBase = origBase
		updater.GitHubRepo = origRepo
	}()

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/system/update/check", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	if hits != 1 {
		t.Errorf("expected 1 upstream request, got %d", hits)
	}
}
