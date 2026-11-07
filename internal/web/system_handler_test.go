package web_test

import (
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStaticPage(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for /, got %d", resp.StatusCode)
	}
}
func TestSystemLogsEndpointReturnsStatsDirectory(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/system/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected system logs endpoint 200, got %d", resp.StatusCode)
	}

	var data model.SystemLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataDir, "stats")
	if data.Path != want {
		t.Fatalf("expected system logs path %q, got %q", want, data.Path)
	}
}
func TestPricingPersistenceUsesDataDir(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := strings.NewReader(`[{
		"model": "gpt-5.5",
		"input_price_per_m": 6,
		"cached_price_per_m": 0.6,
		"cache_creation_price_per_m": 6,
		"output_price_per_m": 36
	}]`)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/pricing", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected pricing update 200, got %d", resp.StatusCode)
	}

	customPricingPath := filepath.Join(dataDir, "custom_pricing.json")
	data, err := os.ReadFile(customPricingPath)
	if err != nil {
		t.Fatalf("expected custom pricing to be written to data-dir: %v", err)
	}
	if !strings.Contains(string(data), `"gpt-5.5"`) {
		t.Fatalf("custom pricing missing updated model: %s", string(data))
	}
}

func TestPricingUpdateRepricesExistingStatsImmediately(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Claude Code",
			Model:        "claude-opus-4-7",
			OutputTokens: 1_000_000,
		},
		75,
		time.Now().UTC(),
		"/claude.jsonl",
		"local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	first := fetchStats(t, srv.URL)
	if allPeriodCost(first) != 75 {
		t.Fatalf("setup expected old persisted cost 75, got %+v", first.Periods)
	}

	body := strings.NewReader(`[{
		"model": "claude-opus-4-7",
		"input_price_per_m": 0,
		"cached_price_per_m": 0,
		"cache_creation_price_per_m": 0,
		"output_price_per_m": 10
	}]`)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/pricing", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected pricing update 200, got %d", resp.StatusCode)
	}

	cost, _, _, _, _, err := database.QueryPeriodStatsAll("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 10 {
		t.Fatalf("expected stored usage cost to be repriced to 10, got %f", cost)
	}

	second := fetchStats(t, srv.URL)
	if allPeriodCost(second) != 10 {
		t.Fatalf("expected cached stats to refresh after repricing, got %+v", second.Periods)
	}
}

func fetchStats(t *testing.T, baseURL string) model.StatsResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected stats 200, got %d", resp.StatusCode)
	}
	var data model.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	return data
}

func allPeriodCost(stats model.StatsResponse) float64 {
	for _, period := range stats.Periods {
		if period.Label == "ALL" {
			return period.Cost
		}
	}
	return -1
}
