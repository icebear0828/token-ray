package web_test

import (
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIStats(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude records
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 5000, OutputTokens: 200},
		1.50, now.Add(-30*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 8000, OutputTokens: 400},
		3.00, now.Add(-2*time.Hour), "/b.jsonl", "local",
	)
	// Gemini records
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-10*time.Minute), "/c.jsonl", "local",
	)

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.StatsResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// Should have time periods
	if len(data.Periods) == 0 {
		t.Fatal("expected periods")
	}

	// Should have sources grouped
	if len(data.Sources) < 2 {
		t.Fatalf("expected at least 2 sources, got %d", len(data.Sources))
	}

	// Find Claude source
	var claude *model.SourceStats
	for i := range data.Sources {
		if data.Sources[i].Name == "Claude Code" {
			claude = &data.Sources[i]
		}
	}
	if claude == nil {
		t.Fatal("Claude Code source not found")
	}
	if claude.TotalInput != 3000 || claude.TotalCached != 13000 || claude.TotalOutput != 600 {
		t.Errorf("Claude totals wrong: %+v", claude)
	}
	if len(claude.Models) != 1 {
		t.Errorf("expected 1 Claude model, got %d", len(claude.Models))
	}
	if len(claude.Models) == 1 {
		modelStats := claude.Models[0]
		if modelStats.InputTokens != 3000 || modelStats.CachedTokens != 13000 || modelStats.OutputTokens != 600 {
			t.Errorf("Claude model token breakdown missing/wrong: %+v", modelStats)
		}
		if modelStats.CacheCreationPricePerM != 6.25 {
			t.Errorf("Claude model cache creation price missing/wrong: %+v", modelStats)
		}
	}

	// Should have devices list
	if len(data.Devices) == 0 {
		t.Error("expected devices list")
	}
}
func TestAPIStatsCodexSourceFilter(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "api", InputTokens: 1000, OutputTokens: 200},
		1.50, now.Add(-2*time.Minute), "/claude.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now.Add(-1*time.Minute), "/codex.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(srv.URL + "/api/stats?device=all&source=Codex")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Periods) != 8 {
		t.Fatalf("expected 8 periods, got %d", len(data.Periods))
	}
	if len(data.Sources) != 1 || data.Sources[0].Name != "Codex" {
		t.Fatalf("expected only Codex source, got %+v", data.Sources)
	}
	if len(data.Projects) != 1 || data.Projects[0].Project != "dashboard" {
		t.Fatalf("expected only Codex project, got %+v", data.Projects)
	}
	if data.Sources[0].TotalInput != 2000 || data.Sources[0].TotalCached != 1500 || data.Sources[0].TotalOutput != 300 {
		t.Fatalf("unexpected Codex token totals: %+v", data.Sources[0])
	}
}
func TestAPIStatsDetailModes(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now.Add(-1*time.Minute), "/codex.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	summaryResp, err := http.Get(srv.URL + "/api/stats?device=all&source=Codex&detail=summary&period=30d")
	if err != nil {
		t.Fatal(err)
	}
	defer summaryResp.Body.Close()
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected summary 200, got %d", summaryResp.StatusCode)
	}
	var summary model.StatsResponse
	if err := json.NewDecoder(summaryResp.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Periods) == 0 || len(summary.Devices) == 0 {
		t.Fatalf("expected summary periods/devices, got %+v", summary)
	}
	if len(summary.Sources) != 1 || len(summary.Sources[0].Models) != 0 || len(summary.Projects) != 0 {
		t.Fatalf("summary should include source totals only, got %+v", summary)
	}
	if len(summary.Charts.DailyCosts) != 30 || len(summary.Charts.CalendarHeatmap) != 30 {
		t.Fatalf("summary should include cost intelligence charts, got %+v", summary.Charts)
	}
	if len(summary.Charts.ToolShare) != 1 || summary.Charts.ToolShare[0].Source != "Codex" || summary.Charts.ToolShare[0].Tokens != 2300 {
		t.Fatalf("summary should include Codex token-share chart data, got %+v", summary.Charts.ToolShare)
	}

	detailsResp, err := http.Get(srv.URL + "/api/stats?device=all&source=Codex&detail=details&period=30d")
	if err != nil {
		t.Fatal(err)
	}
	defer detailsResp.Body.Close()
	if detailsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected details 200, got %d", detailsResp.StatusCode)
	}
	var details model.StatsResponse
	if err := json.NewDecoder(detailsResp.Body).Decode(&details); err != nil {
		t.Fatal(err)
	}
	if len(details.Periods) != 0 || len(details.Devices) != 0 {
		t.Fatalf("details should omit summary periods/devices, got %+v", details)
	}
	if len(details.Charts.DailyCosts) != 0 || len(details.Charts.CalendarHeatmap) != 0 || len(details.Charts.ToolShare) != 0 {
		t.Fatalf("details should omit cost intelligence charts, got %+v", details.Charts)
	}
	if len(details.Sources) != 1 || len(details.Sources[0].Models) != 1 || len(details.Projects) != 1 {
		t.Fatalf("expected details models/projects, got %+v", details)
	}

	badResp, err := http.Get(srv.URL + "/api/stats?detail=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid detail 400, got %d", badResp.StatusCode)
	}
}
func TestAPIStatsPeriodFilterAndGranularity(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "recent", InputTokens: 1000, OutputTokens: 100},
		1.00, now.Add(-1*time.Hour), "/recent.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "old", InputTokens: 5000, OutputTokens: 500},
		5.00, now.AddDate(0, 0, -40), "/old.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	detailsResp, err := http.Get(srv.URL + "/api/stats?detail=details&period=7d")
	if err != nil {
		t.Fatal(err)
	}
	defer detailsResp.Body.Close()
	var details model.StatsResponse
	if err := json.NewDecoder(detailsResp.Body).Decode(&details); err != nil {
		t.Fatal(err)
	}
	if len(details.Projects) != 1 || details.Projects[0].Project != "recent" {
		t.Fatalf("period=7d should exclude the 40-day-old project, got %+v", details.Projects)
	}

	summaryResp, err := http.Get(srv.URL + "/api/stats?detail=summary&period=24h")
	if err != nil {
		t.Fatal(err)
	}
	defer summaryResp.Body.Close()
	var summary model.StatsResponse
	if err := json.NewDecoder(summaryResp.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if summary.Charts.Granularity != "hourly" {
		t.Fatalf("expected hourly granularity for 24h, got %q", summary.Charts.Granularity)
	}
	if len(summary.Charts.DailyCosts) != 24 {
		t.Fatalf("expected 24 hourly buckets for 24h, got %d", len(summary.Charts.DailyCosts))
	}

	badResp, err := http.Get(srv.URL + "/api/stats?period=banana")
	if err != nil {
		t.Fatal(err)
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid period 400, got %d", badResp.StatusCode)
	}
}

func TestAPICacheSavings(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude: 1000 input (800 cached), 200 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 800, OutputTokens: 200},
		0, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	// Gemini: 500 input (0 cached), 100 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/cache-savings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.CacheSavingsResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// actual_cost: calculator computes real cost with cached pricing
	// hypothetical_cost: calculator computes cost as if cached_tokens were charged at full input price
	if data.HypotheticalCost <= data.ActualCost {
		t.Errorf("hypothetical should exceed actual: hypo=%f actual=%f", data.HypotheticalCost, data.ActualCost)
	}
	if data.Saved < 0 {
		t.Errorf("saved should be >= 0, got %f", data.Saved)
	}
	if data.CacheHitRate < 0 || data.CacheHitRate > 100 {
		t.Errorf("cache hit rate should be 0-100, got %f", data.CacheHitRate)
	}
}

func TestAPIStatsETagAnd304NotModified(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", OutputTokens: 1_000_000},
		25.00, now.Add(-5*time.Minute), "/a.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	// First request returns 200 with ETag and Cache-Control: no-cache.
	resp1, err := client.Get(srv.URL + "/api/stats?period=30d")
	if err != nil {
		t.Fatal(err)
	}
	body1, err := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}
	etag := resp1.Header.Get("ETag")
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Fatalf("expected quoted ETag header, got %q", etag)
	}
	if cc := resp1.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("expected Cache-Control: no-cache, got %q", cc)
	}
	if len(body1) == 0 {
		t.Fatal("expected non-empty JSON body")
	}

	// Exact, weak W/, comma-separated list, and wildcard If-None-Match all return 304 with 0 body bytes.
	for _, inm := range []string{
		etag,
		"W/" + etag,
		`"other-tag", W/` + etag + `, "another"`,
		"*",
	} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/stats?period=30d", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("If-None-Match", inm)
		req.Header.Set("Accept-Encoding", "gzip")
		resp304, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body304, err := io.ReadAll(resp304.Body)
		resp304.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp304.StatusCode != http.StatusNotModified {
			t.Fatalf("If-None-Match=%q expected 304, got %d", inm, resp304.StatusCode)
		}
		if len(body304) != 0 {
			t.Fatalf("If-None-Match=%q expected 0 body bytes on 304, got %d", inm, len(body304))
		}
		if resp304.Header.Get("Content-Encoding") != "" {
			t.Fatalf("304 must not set Content-Encoding, got %q", resp304.Header.Get("Content-Encoding"))
		}
		if resp304.Header.Get("ETag") != etag {
			t.Fatalf("304 expected ETag %q, got %q", etag, resp304.Header.Get("ETag"))
		}
		if resp304.Header.Get("Cache-Control") != "no-cache" {
			t.Fatalf("304 expected Cache-Control no-cache, got %q", resp304.Header.Get("Cache-Control"))
		}
	}

	// Stale/mismatched If-None-Match returns 200 with full JSON.
	reqMiss, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/stats?period=30d", nil)
	reqMiss.Header.Set("If-None-Match", `"stale-etag"`)
	respMiss, err := client.Do(reqMiss)
	if err != nil {
		t.Fatal(err)
	}
	missBody, _ := io.ReadAll(respMiss.Body)
	respMiss.Body.Close()
	if respMiss.StatusCode != http.StatusOK || len(missBody) == 0 {
		t.Fatalf("expected 200 with body for mismatched If-None-Match, got status=%d len=%d", respMiss.StatusCode, len(missBody))
	}

	// Updating pricing clears StatsCache and changes the ETag.
	putReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/pricing", strings.NewReader(`[{
		"model": "claude-opus-4-7",
		"input_price_per_m": 0,
		"cached_price_per_m": 0,
		"cache_creation_price_per_m": 0,
		"output_price_per_m": 10
	}]`))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected pricing PUT 200, got %d", putResp.StatusCode)
	}

	reqAfter, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/stats?period=30d", nil)
	reqAfter.Header.Set("If-None-Match", etag)
	respAfter, err := client.Do(reqAfter)
	if err != nil {
		t.Fatal(err)
	}
	bodyAfter, _ := io.ReadAll(respAfter.Body)
	respAfter.Body.Close()
	if respAfter.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after cache invalidation, got %d", respAfter.StatusCode)
	}
	if respAfter.Header.Get("ETag") == etag {
		t.Fatalf("expected new ETag after pricing update, still got %q", etag)
	}
	var updated model.StatsResponse
	if err := json.Unmarshal(bodyAfter, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Periods[len(updated.Periods)-1].Cost != 10 {
		t.Fatalf("expected repriced cost 10, got %+v", updated.Periods)
	}
}

func TestHTTPGzipMiddleware(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Disable automatic decompression so we verify real wire gzip headers & framing.
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	// 1. /api/stats with Accept-Encoding: gzip compresses and round-trips.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/stats?period=30d", nil)
	req.Header.Set("Accept-Encoding", "br, gzip;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		resp.Body.Close()
		t.Fatalf("expected Content-Encoding: gzip, got %q", resp.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(resp.Header.Get("Vary"), "Accept-Encoding") {
		resp.Body.Close()
		t.Fatalf("expected Vary to include Accept-Encoding, got %q", resp.Header.Get("Vary"))
	}
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		resp.Body.Close()
		t.Fatalf("open gzip reader: %v", err)
	}
	decompressed, err := io.ReadAll(gzReader)
	gzReader.Close()
	resp.Body.Close()
	if err != nil {
		t.Fatalf("decompress gzip body: %v", err)
	}
	var stats model.StatsResponse
	if err := json.Unmarshal(decompressed, &stats); err != nil {
		t.Fatalf("unmarshal decompressed stats: %v", err)
	}
	if len(stats.Periods) != 8 {
		t.Fatalf("expected 8 periods after gzip round-trip, got %d", len(stats.Periods))
	}

	// 2. Static "/" with Accept-Encoding: gzip compresses cleanly.
	staticReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	staticReq.Header.Set("Accept-Encoding", "gzip")
	staticResp, err := client.Do(staticReq)
	if err != nil {
		t.Fatal(err)
	}
	if staticResp.StatusCode != http.StatusOK {
		staticResp.Body.Close()
		t.Fatalf("expected 200 for /, got %d", staticResp.StatusCode)
	}
	if staticResp.Header.Get("Content-Encoding") != "gzip" {
		staticResp.Body.Close()
		t.Fatalf("expected Content-Encoding: gzip for /, got %q", staticResp.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(staticResp.Header.Get("Vary"), "Accept-Encoding") {
		staticResp.Body.Close()
		t.Fatalf("expected Vary: Accept-Encoding for /, got %q", staticResp.Header.Get("Vary"))
	}
	staticGZ, err := gzip.NewReader(staticResp.Body)
	if err != nil {
		staticResp.Body.Close()
		t.Fatalf("open static gzip reader: %v", err)
	}
	staticHTML, err := io.ReadAll(staticGZ)
	staticGZ.Close()
	staticResp.Body.Close()
	if err != nil || !strings.Contains(strings.ToLower(string(staticHTML)), "<html") {
		t.Fatalf("expected valid HTML after static gzip round-trip, err=%v len=%d", err, len(staticHTML))
	}

	// 3. Explicit q=0 or omitted Accept-Encoding returns plain JSON with Vary.
	plainReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/stats?period=30d", nil)
	plainReq.Header.Set("Accept-Encoding", "gzip;q=0")
	plainResp, err := client.Do(plainReq)
	if err != nil {
		t.Fatal(err)
	}
	plainBody, _ := io.ReadAll(plainResp.Body)
	plainResp.Body.Close()
	if plainResp.Header.Get("Content-Encoding") != "" {
		t.Fatalf("expected no Content-Encoding for gzip;q=0, got %q", plainResp.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(plainResp.Header.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("expected Vary: Accept-Encoding even uncompressed, got %q", plainResp.Header.Get("Vary"))
	}
	if err := json.Unmarshal(plainBody, &stats); err != nil {
		t.Fatalf("expected plain JSON for gzip;q=0: %v", err)
	}

	// 4. /download/ skips gzip middleware.
	dlReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/download/dashboard", nil)
	dlReq.Header.Set("Accept-Encoding", "gzip")
	dlResp, err := client.Do(dlReq)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, dlResp.Body)
	dlResp.Body.Close()
	if dlResp.Header.Get("Content-Encoding") != "" {
		t.Fatalf("/download/ must skip gzip, got Content-Encoding=%q", dlResp.Header.Get("Content-Encoding"))
	}

	// 5. Empty 201 response (POST /api/track) does not emit gzip stream or header.
	trackReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/track", strings.NewReader(`{
		"device_id": "remote",
		"usage": {"source": "Claude Code", "model": "claude-opus-4-7", "input_tokens": 10, "output_tokens": 5}
	}`))
	trackReq.Header.Set("Accept-Encoding", "gzip")
	trackReq.Header.Set("Content-Type", "application/json")
	trackResp, err := client.Do(trackReq)
	if err != nil {
		t.Fatal(err)
	}
	trackBody, _ := io.ReadAll(trackResp.Body)
	trackResp.Body.Close()
	if trackResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for /api/track, got %d", trackResp.StatusCode)
	}
	if len(trackBody) != 0 || trackResp.Header.Get("Content-Encoding") != "" {
		t.Fatalf("empty 201 response must not be gzipped: len=%d encoding=%q", len(trackBody), trackResp.Header.Get("Content-Encoding"))
	}
}

func TestAPIStatsCacheInvalidatedByMutations(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", OutputTokens: 1000, UUID: "seed-1"},
		1.00, now.Add(-5*time.Minute), "/a.jsonl", "dev-a",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	fetchSummary := func(ifNoneMatch string) (int, string, model.StatsResponse) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/stats?detail=summary&period=30d", nil)
		if err != nil {
			t.Fatal(err)
		}
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var parsed model.StatsResponse
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatalf("unmarshal stats: %v", err)
			}
		}
		return resp.StatusCode, resp.Header.Get("ETag"), parsed
	}

	// Warm StatsCache.
	status, etag1, s1 := fetchSummary("")
	if status != http.StatusOK || etag1 == "" || len(s1.Devices) != 1 {
		t.Fatalf("unexpected initial response: status=%d etag=%q devices=%+v", status, etag1, s1.Devices)
	}

	// 1. POST /api/track invalidates StatsCache immediately.
	trackResp, err := client.Post(srv.URL+"/api/track", "application/json", strings.NewReader(`{
		"device_id": "dev-b",
		"usage": {"source": "Codex", "model": "gpt-5.5", "input_tokens": 1000, "output_tokens": 500, "uuid": "track-2"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	trackResp.Body.Close()
	if trackResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from /api/track, got %d", trackResp.StatusCode)
	}

	status, etag2, s2 := fetchSummary(etag1)
	if status != http.StatusOK || etag2 == etag1 || len(s2.Devices) != 2 {
		t.Fatalf("expected immediate cache invalidation after /api/track: status=%d etag1=%q etag2=%q devices=%+v", status, etag1, etag2, s2.Devices)
	}

	// 2. POST /api/device-alias invalidates StatsCache immediately.
	aliasResp, err := client.Post(srv.URL+"/api/device-alias", "application/json", strings.NewReader(`{"device_id":"dev-b","display_name":"Studio Mac"}`))
	if err != nil {
		t.Fatal(err)
	}
	aliasResp.Body.Close()
	if aliasResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from POST /api/device-alias, got %d", aliasResp.StatusCode)
	}

	status, etag3, s3 := fetchSummary(etag2)
	if status != http.StatusOK || etag3 == etag2 {
		t.Fatalf("expected immediate cache invalidation after POST /api/device-alias: status=%d etag2=%q etag3=%q", status, etag2, etag3)
	}
	foundAlias := false
	for _, dev := range s3.Devices {
		if dev.ID == "dev-b" && dev.DisplayName == "Studio Mac" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Fatalf("expected Studio Mac alias in stats, got %+v", s3.Devices)
	}

	// 3. DELETE /api/device-alias invalidates StatsCache immediately.
	delAliasReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/device-alias?device_id=dev-b", nil)
	delAliasResp, err := client.Do(delAliasReq)
	if err != nil {
		t.Fatal(err)
	}
	delAliasResp.Body.Close()
	if delAliasResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from DELETE /api/device-alias, got %d", delAliasResp.StatusCode)
	}

	status, etag4, s4 := fetchSummary(etag3)
	if status != http.StatusOK || etag4 == etag3 {
		t.Fatalf("expected immediate cache invalidation after DELETE /api/device-alias: status=%d etag3=%q etag4=%q", status, etag3, etag4)
	}
	for _, dev := range s4.Devices {
		if dev.ID == "dev-b" && dev.DisplayName != "dev-b" {
			t.Fatalf("expected alias cleared in stats, got %+v", dev)
		}
	}

	// 4. DELETE /api/devices invalidates StatsCache immediately.
	delDevReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/devices?device_id=dev-b", nil)
	delDevResp, err := client.Do(delDevReq)
	if err != nil {
		t.Fatal(err)
	}
	delDevResp.Body.Close()
	if delDevResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from DELETE /api/devices, got %d", delDevResp.StatusCode)
	}

	status, etag5, s5 := fetchSummary(etag4)
	if status != http.StatusOK || etag5 == etag4 || len(s5.Devices) != 1 {
		t.Fatalf("expected immediate cache invalidation after DELETE /api/devices: status=%d etag4=%q etag5=%q devices=%+v", status, etag4, etag5, s5.Devices)
	}
}
