package dashboard_test

import (
	"sync"
	"testing"
	"time"

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
