package model_test

import (
	"testing"

	"token-ray/internal/model"
)

func TestCacheHitRatePercent(t *testing.T) {
	tests := []struct {
		name         string
		inputTokens  int
		cachedTokens int
		want         float64
	}{
		{name: "calculates cached share", inputTokens: 1000, cachedTokens: 250, want: 25},
		{name: "returns zero without input tokens", inputTokens: 0, cachedTokens: 250, want: 0},
		{name: "returns zero without cached tokens", inputTokens: 1000, cachedTokens: 0, want: 0},
		{name: "caps impossible rates", inputTokens: 100, cachedTokens: 250, want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.CacheHitRatePercent(tt.inputTokens, tt.cachedTokens)
			if got != tt.want {
				t.Fatalf("expected %.1f, got %.1f", tt.want, got)
			}
		})
	}
}
