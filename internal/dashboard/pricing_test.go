package dashboard_test

import (
	"testing"

	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/testutil"
)

func TestBuildPricingEntriesSortedWithCacheCreationPrice(t *testing.T) {
	calc := testutil.NewTestCalc(t)

	entries := dashboard.BuildPricingEntries(calc)
	if len(entries) == 0 {
		t.Fatal("expected pricing entries")
	}

	for i := 1; i < len(entries); i++ {
		if entries[i-1].Model >= entries[i].Model {
			t.Fatalf("entries not sorted by model: %q before %q", entries[i-1].Model, entries[i].Model)
		}
	}

	var found bool
	for _, entry := range entries {
		if entry.Model != "claude-opus-4-7" {
			continue
		}
		found = true
		assertApprox(t, entry.InputPricePerM, 5)
		assertApprox(t, entry.CachedPricePerM, 0.5)
		assertApprox(t, entry.CacheCreationPricePerM, 6.25)
		assertApprox(t, entry.OutputPricePerM, 25)
	}
	if !found {
		t.Fatal("expected claude-opus-4-7 pricing entry")
	}
}
