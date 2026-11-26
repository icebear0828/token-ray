package web_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/web"
)

const benchPricingJSON = `{
  "models": {
    "claude-opus-4-7": {"input_price_per_m": 5, "cached_price_per_m": 0.5, "cache_creation_price_per_m": 6.25, "output_price_per_m": 25},
    "claude-sonnet-4-6": {"input_price_per_m": 3, "cached_price_per_m": 0.3, "cache_creation_price_per_m": 3.75, "output_price_per_m": 15},
    "gpt-5.5": {"input_price_per_m": 5, "cached_price_per_m": 0.5, "cache_creation_price_per_m": 0, "output_price_per_m": 30},
    "gpt-5.4-mini": {"input_price_per_m": 0.75, "cached_price_per_m": 0.075, "cache_creation_price_per_m": 0, "output_price_per_m": 4.5},
    "gemini-2.5-pro": {"input_price_per_m": 1.25, "cached_price_per_m": 0.125, "cache_creation_price_per_m": 0, "output_price_per_m": 10},
    "gemini-3-flash": {"input_price_per_m": 0.5, "cached_price_per_m": 0.05, "cache_creation_price_per_m": 0, "output_price_per_m": 3},
    "deepseek-chat": {"input_price_per_m": 0.27, "cached_price_per_m": 0.027, "cache_creation_price_per_m": 0, "output_price_per_m": 1.1},
    "deepseek-reasoner": {"input_price_per_m": 0.55, "cached_price_per_m": 0.055, "cache_creation_price_per_m": 0, "output_price_per_m": 2.19}
  }
}`

func seedWebBenchEnv(b *testing.B, rowCount int) (*db.DB, *calculator.Calculator) {
	b.Helper()
	database, err := db.New(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("open bench db: %v", err)
	}
	b.Cleanup(func() { _ = database.Close() })

	calc, err := calculator.NewFromBytes([]byte(benchPricingJSON))
	if err != nil {
		b.Fatalf("init bench calc: %v", err)
	}

	now := time.Now().UTC()
	devices := []string{"local", "mac-studio", "linux-worker"}
	sources := []string{"Claude Code", "Codex", "Gemini CLI", "DeepSeek Harness"}
	models := []string{
		"claude-opus-4-7",
		"claude-sonnet-4-6",
		"gpt-5.5",
		"gpt-5.4-mini",
		"gemini-2.5-pro",
		"gemini-3-flash",
		"deepseek-chat",
		"deepseek-reasoner",
	}
	projects := []string{
		"token-ray", "agent-core", "web-ui", "infra", "cli",
		"evals", "docs", "sandbox", "ml-pipeline", "gateway",
	}

	err = database.RunInTx(func(tx *db.Tx) error {
		for i := 0; i < rowCount; i++ {
			age := time.Duration(i%8760) * time.Hour
			if i%20 == 0 {
				age = time.Duration(i%45) * time.Minute
			}
			ts := now.Add(-age)
			src := sources[i%len(sources)]
			mdl := models[i%len(models)]
			proj := projects[i%len(projects)]
			dev := devices[i%len(devices)]

			var durMS int64
			var timedOut int
			outTok := 100 + (i%50)*20
			if i%3 == 0 {
				durMS = int64(800 + (i%10)*250)
				timedOut = outTok
			}

			u := model.TokenUsage{
				Source:              src,
				Model:               mdl,
				Project:             proj,
				InputTokens:         1000 + (i%20)*250,
				CachedTokens:        400 + (i%10)*100,
				CacheCreationTokens: 50 + (i%5)*25,
				OutputTokens:        outTok,
				DurationMS:          durMS,
				TimedOutputTokens:   timedOut,
				UUID:                fmt.Sprintf("web-bench-uuid-%d", i),
			}
			cost, _ := calc.CalculateCost(mdl, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
			if err := tx.InsertUsageWithTime(u, cost, ts, fmt.Sprintf("/tmp/%s/%s/session-%d.jsonl", dev, proj, i%40), dev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatalf("seed web bench rows: %v", err)
	}

	if err := database.RawExec(`
		WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 1000)
		INSERT INTO usage_records (log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, uuid, superseded)
		SELECT ?, 'Claude Code', 'claude-opus-4-7', 'superseded-proj', 500, 100, 0, 50, 0.5, '/tmp/sup.jsonl', 'local', 'web-sup-' || x, 1
		FROM cnt
	`, now.Add(-2*time.Hour).Format(time.RFC3339Nano)); err != nil {
		b.Fatalf("seed web superseded rows: %v", err)
	}

	return database, calc
}

func BenchmarkBuildStatsSummary(b *testing.B) {
	database, calc := seedWebBenchEnv(b, 20000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := dashboard.BuildStatsSummary(database, calc, "all", "", "30d", false)
		if err != nil {
			b.Fatal(err)
		}
		if len(resp.Periods) == 0 {
			b.Fatal("expected periods")
		}
	}
}

func BenchmarkBuildStatsDetails(b *testing.B) {
	database, calc := seedWebBenchEnv(b, 20000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := dashboard.BuildStatsDetails(database, calc, "all", "", "30d", false)
		if err != nil {
			b.Fatal(err)
		}
		if len(resp.Sources) == 0 {
			b.Fatal("expected sources")
		}
	}
}

func BenchmarkGetAPIStats(b *testing.B) {
	database, calc := seedWebBenchEnv(b, 20000)

	b.Run("CacheHit", func(b *testing.B) {
		handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
		// Warm cache
		warmReq := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
		warmRec := httptest.NewRecorder()
		handler.ServeHTTP(warmRec, warmReq)
		if warmRec.Code != http.StatusOK {
			b.Fatalf("warmup status = %d", warmRec.Code)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("status = %d", rec.Code)
			}
			_, _ = io.Copy(io.Discard, rec.Body)
		}
	})

	b.Run("CacheHitGzip", func(b *testing.B) {
		handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
		warmReq := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
		warmReq.Header.Set("Accept-Encoding", "gzip")
		warmRec := httptest.NewRecorder()
		handler.ServeHTTP(warmRec, warmReq)
		if warmRec.Code != http.StatusOK {
			b.Fatalf("warmup status = %d", warmRec.Code)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("status = %d", rec.Code)
			}
			_, _ = io.Copy(io.Discard, rec.Body)
		}
	})

	b.Run("ETag304", func(b *testing.B) {
		handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
		warmReq := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
		warmRec := httptest.NewRecorder()
		handler.ServeHTTP(warmRec, warmReq)
		etag := warmRec.Header().Get("ETag")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
			if etag != "" {
				req.Header.Set("If-None-Match", etag)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK && rec.Code != http.StatusNotModified {
				b.Fatalf("status = %d", rec.Code)
			}
		}
	})

	b.Run("CacheMiss", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
			req := httptest.NewRequest(http.MethodGet, "/api/stats?period=30d", nil)
			rec := httptest.NewRecorder()
			b.StartTimer()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("status = %d", rec.Code)
			}
		}
	})
}
