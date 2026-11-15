package db_test

import (
	"token-ray/internal/db"
	"testing"
)

func assertSingleSourceTotal(t *testing.T, database *db.DB, deviceID string, source string, events int, input int, cached int, cacheCreation int, output int, cost float64) {
	t.Helper()
	stats, err := database.QuerySourceTotalsSummary(deviceID, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected one source total, got %+v", stats)
	}
	got := stats[0]
	if got.Source != source || got.Events != events || got.InputTokens != input || got.CachedTokens != cached || got.CacheCreationTokens != cacheCreation || got.OutputTokens != output || got.TotalCost != cost {
		t.Fatalf("unexpected source total: got %+v want source=%s events=%d input=%d cached=%d cacheCreation=%d output=%d cost=%f", got, source, events, input, cached, cacheCreation, output, cost)
	}
}
func assertSourceTotalsEqual(t *testing.T, got []db.SourceTotalStat, want []db.SourceTotalStat) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("source totals length mismatch: got %+v want %+v", got, want)
	}
	gotBySource := make(map[string]db.SourceTotalStat, len(got))
	for _, stat := range got {
		gotBySource[stat.Source] = stat
	}
	for _, expected := range want {
		actual, ok := gotBySource[expected.Source]
		if !ok {
			t.Fatalf("missing source %s in totals: got %+v want %+v", expected.Source, got, want)
		}
		if actual != expected {
			t.Fatalf("source total mismatch for %s: got %+v want %+v", expected.Source, actual, expected)
		}
	}
}
