package dashboard_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/model"
)

func TestStatsCacheReusesFreshValueAndReturnsCopies(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cache := dashboard.NewStatsCacheWithClock(2*time.Second, func() time.Time { return now })
	key := dashboard.StatsCacheKey{DeviceID: "all", Source: "Codex", IsPaused: false}

	builds := 0
	build := func() (*model.StatsResponse, error) {
		builds++
		return &model.StatsResponse{
			Periods: []model.PeriodCost{{Label: "ALL", InputTokens: builds}},
			Charts: model.StatsCharts{
				DailyCosts:      []model.DailyCostBucket{{Date: "2026-06-08", Cost: 1.25}},
				CalendarHeatmap: []model.CalendarHeatmapBucket{{Date: "2026-06-08", Tokens: 1050}},
				ToolShare:       []model.ToolShareBucket{{Source: "Codex", Tokens: 1050}},
			},
			Sources: []model.SourceStats{{
				Name: "Codex",
				Models: []model.ModelStats{{
					Model: "gpt-5.5",
				}},
			}},
			Devices:  []model.DeviceInfo{{ID: "local", DisplayName: "local"}},
			Projects: []model.ProjectStat{{Project: "token"}},
			IsPaused: key.IsPaused,
		}, nil
	}

	first, err := cache.Get(key, build)
	if err != nil {
		t.Fatal(err)
	}
	first.Periods[0].Label = "mutated"
	first.Charts.DailyCosts[0].Date = "mutated"
	first.Charts.CalendarHeatmap[0].Date = "mutated"
	first.Charts.ToolShare[0].Source = "mutated"
	first.Sources[0].Models[0].Model = "mutated"

	second, err := cache.Get(key, build)
	if err != nil {
		t.Fatal(err)
	}

	if builds != 1 {
		t.Fatalf("expected one build within TTL, got %d", builds)
	}
	if second.Periods[0].Label != "ALL" || second.Sources[0].Models[0].Model != "gpt-5.5" || second.Charts.DailyCosts[0].Date != "2026-06-08" || second.Charts.CalendarHeatmap[0].Date != "2026-06-08" || second.Charts.ToolShare[0].Source != "Codex" {
		t.Fatalf("expected cached copies to be isolated from caller mutation, got %+v", second)
	}
}

func TestStatsCacheExpiresAndKeysByFilter(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cache := dashboard.NewStatsCacheWithClock(2*time.Second, func() time.Time { return now })
	key := dashboard.StatsCacheKey{DeviceID: "all", Source: "Codex"}

	builds := 0
	build := func() (*model.StatsResponse, error) {
		builds++
		return &model.StatsResponse{Periods: []model.PeriodCost{{Label: "ALL", InputTokens: builds}}}, nil
	}

	if _, err := cache.Get(key, build); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(dashboard.StatsCacheKey{DeviceID: "all", Source: "Claude Code"}, build); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Second)
	third, err := cache.Get(key, build)
	if err != nil {
		t.Fatal(err)
	}

	if builds != 3 {
		t.Fatalf("expected separate filter and TTL expiry to rebuild, got %d builds", builds)
	}
	if third.Periods[0].InputTokens != 3 {
		t.Fatalf("expected expired key to rebuild third value, got %+v", third.Periods[0])
	}
}

func TestStatsCacheCoalescesConcurrentBuilds(t *testing.T) {
	cache := dashboard.NewStatsCache(2 * time.Second)
	key := dashboard.StatsCacheKey{DeviceID: "all", Source: "Codex"}

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	builds := 0
	build := func() (*model.StatsResponse, error) {
		builds++
		once.Do(func() { close(started) })
		<-release
		return &model.StatsResponse{Periods: []model.PeriodCost{{Label: "ALL", InputTokens: 42}}}, nil
	}

	const callers = 5
	var wg sync.WaitGroup
	wg.Add(callers)
	errs := make(chan error, callers)
	results := make(chan *model.StatsResponse, callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			stats, err := cache.Get(key, build)
			if err != nil {
				errs <- err
				return
			}
			results <- stats
		}()
	}

	<-started
	close(release)
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("expected one coalesced build, got %d", builds)
	}
	for stats := range results {
		if stats.Periods[0].InputTokens != 42 {
			t.Fatalf("unexpected cached stats: %+v", stats)
		}
	}
}

func TestStatsCacheGetJSONCachesBytesAndETag(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cache := dashboard.NewStatsCacheWithClock(2*time.Second, func() time.Time { return now })
	key := dashboard.StatsCacheKey{DeviceID: "all", Source: "Codex", Period: "30d", Detail: "summary"}

	builds := 0
	build := func() (*model.StatsResponse, error) {
		builds++
		return &model.StatsResponse{
			Periods: []model.PeriodCost{{Label: "ALL", InputTokens: builds * 100}},
			Sources: []model.SourceStats{{Name: "Codex", TotalInput: builds * 100}},
		}, nil
	}

	raw1, etag1, err := cache.GetJSON(key, build)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(etag1, `"`) || !strings.HasSuffix(etag1, `"`) || len(etag1) < 10 {
		t.Fatalf("expected quoted strong ETag, got %q", etag1)
	}

	var decoded model.StatsResponse
	if err := json.Unmarshal(raw1, &decoded); err != nil {
		t.Fatalf("unmarshal cached json: %v", err)
	}
	if len(decoded.Periods) != 1 || decoded.Periods[0].InputTokens != 100 {
		t.Fatalf("unexpected decoded payload: %+v", decoded)
	}

	raw2, etag2, err := cache.GetJSON(key, build)
	if err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("expected one build within TTL, got %d", builds)
	}
	if etag2 != etag1 {
		t.Fatalf("expected identical ETag on cache hit, got %q vs %q", etag2, etag1)
	}
	if unsafe.SliceData(raw1) != unsafe.SliceData(raw2) {
		t.Fatal("expected GetJSON cache hit to reuse immutable backing slice without re-marshal")
	}

	// Struct Get() on the same warm key should also hit without rebuilding.
	structCopy, err := cache.Get(key, build)
	if err != nil {
		t.Fatal(err)
	}
	if builds != 1 || structCopy.Periods[0].InputTokens != 100 {
		t.Fatalf("expected Get() to reuse warm entry, builds=%d stats=%+v", builds, structCopy)
	}

	// Advancing past TTL triggers rebuild and produces a new ETag.
	now = now.Add(3 * time.Second)
	raw3, etag3, err := cache.GetJSON(key, build)
	if err != nil {
		t.Fatal(err)
	}
	if builds != 2 {
		t.Fatalf("expected rebuild after TTL expiry, got %d", builds)
	}
	if etag3 == etag1 {
		t.Fatalf("expected new ETag after payload change, still got %q", etag3)
	}
	if err := json.Unmarshal(raw3, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Periods[0].InputTokens != 200 {
		t.Fatalf("expected rebuilt payload 200, got %+v", decoded)
	}

	// Clear() invalidates cached JSON too.
	cache.Clear()
	_, _, err = cache.GetJSON(key, build)
	if err != nil {
		t.Fatal(err)
	}
	if builds != 3 {
		t.Fatalf("expected rebuild after Clear(), got %d", builds)
	}
}

func TestStatsCacheGetJSONCoalescesConcurrentBuilds(t *testing.T) {
	cache := dashboard.NewStatsCache(2 * time.Second)
	key := dashboard.StatsCacheKey{DeviceID: "all", Period: "7d"}

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	builds := 0
	build := func() (*model.StatsResponse, error) {
		builds++
		once.Do(func() { close(started) })
		<-release
		return &model.StatsResponse{Periods: []model.PeriodCost{{Label: "7d", InputTokens: 77}}}, nil
	}

	const callers = 6
	var wg sync.WaitGroup
	wg.Add(callers)
	type result struct {
		raw  []byte
		etag string
		err  error
	}
	results := make(chan result, callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			raw, etag, err := cache.GetJSON(key, build)
			results <- result{raw: raw, etag: etag, err: err}
		}()
	}

	<-started
	close(release)
	wg.Wait()
	close(results)

	if builds != 1 {
		t.Fatalf("expected 1 coalesced build, got %d", builds)
	}
	var firstRaw []byte
	var firstETag string
	for res := range results {
		if res.err != nil {
			t.Fatal(res.err)
		}
		if firstRaw == nil {
			firstRaw = res.raw
			firstETag = res.etag
			continue
		}
		if res.etag != firstETag || unsafe.SliceData(res.raw) != unsafe.SliceData(firstRaw) {
			t.Fatalf("expected all concurrent callers to share marshaled bytes and ETag")
		}
	}
}
