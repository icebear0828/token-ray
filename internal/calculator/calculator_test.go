package calculator_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ai-flight-dashboard/internal/calculator"
)

func TestCalculateCost(t *testing.T) {
	// Create a temporary pricing table
	tempDir := t.TempDir()
	pricingPath := filepath.Join(tempDir, "pricing_table.json")
	content := `{
		"models": {
			"test-model": {
				"input_price_per_m": 2.00,
				"cached_price_per_m": 0.50,
				"cache_creation_price_per_m": 3.00,
				"output_price_per_m": 10.00
			}
		}
	}`
	err := os.WriteFile(pricingPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp pricing table: %v", err)
	}

	calc, err := calculator.New(pricingPath)
	if err != nil {
		t.Fatalf("Failed to initialize calculator: %v", err)
	}

	// 1,000,000 input tokens = $2.00
	// 1,000,000 output tokens = $10.00
	cost, err := calc.CalculateCost("test-model", 1000000, 0, 0, 1000000)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected := 12.00
	if cost != expected {
		t.Errorf("Expected cost %f, got %f", expected, cost)
	}

	// Test cached tokens
	// 500,000 input, 500,000 cached, 0 cache_creation, 1,000,000 output
	// baseInput: max(500000 - 500000 - 0, 0) = 0 → inputCost = $0.00
	// cached: 500,000 * 0.5 / 1000000 = $0.25
	// output: 1000000 * 10.0 / 1000000 = $10.00
	// total = 10.25
	cost, err = calc.CalculateCost("test-model", 500000, 500000, 0, 1000000)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected = 10.25
	if cost != expected {
		t.Errorf("Expected cost %f, got %f", expected, cost)
	}

	// Test cache creation tokens
	// 600,000 input, 400,000 cached, 100,000 cache creation, 1,000,000 output
	// baseInput: max(600000 - 400000 - 100000, 0) = 100,000 → inputCost = $0.20
	// cached: 400,000 * 0.5 / 1000000 = $0.20
	// cache_creation: 100,000 * 3.0 / 1000000 = $0.30
	// output: 1000000 * 10.0 / 1000000 = $10.00
	// total = 10.70
	cost, err = calc.CalculateCost("test-model", 600000, 400000, 100000, 1000000)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected = 10.70
	if cost != expected {
		t.Errorf("Expected cost %f, got %f", expected, cost)
	}

	// Unknown model → returns 0 cost, no error (graceful fallback)
	cost, err = calc.CalculateCost("unknown", 100, 0, 0, 100)
	if err != nil {
		t.Errorf("Unexpected error for unknown model: %v", err)
	}
	if cost != 0 {
		t.Errorf("Expected 0 cost for unknown model, got %f", cost)
	}
}

func TestNormalizeModelName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"claude-sonnet-4-5", "claude-sonnet-4-5"},
		{"claude-sonnet-4-5-thinking", "claude-sonnet-4-5"},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5"},
		{"claude-sonnet-4-5-thinking-20251101", "claude-sonnet-4-5"},
		{"gemini-3.1-flash-lite-preview", "gemini-3.1-flash-lite"},
		{"claude-opus-4-5-20260101", "claude-opus-4-5"},
		{"unknown-model-20991231", "unknown-model"},
		{"no-suffix", "no-suffix"},
		{"gemini-2.0-flash-thinking-preview", "gemini-2.0-flash"},
		{"gemini-2.0-flash-preview-thinking", "gemini-2.0-flash"},
		{"claude-sonnet-4-5-preview-thinking-20251101", "claude-sonnet-4-5"},
	}
	for _, tc := range cases {
		got := calculator.NormalizeModelName(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeModelName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCalculateCostFallbackNormalization(t *testing.T) {
	tempDir := t.TempDir()
	pricingPath := filepath.Join(tempDir, "pricing_table.json")
	content := `{
		"models": {
			"claude-sonnet-4-5": {
				"input_price_per_m": 3.00,
				"cached_price_per_m": 0.30,
				"cache_creation_price_per_m": 3.75,
				"output_price_per_m": 15.00
			},
			"gemini-3.1-flash-lite": {
				"input_price_per_m": 0.10,
				"cached_price_per_m": 0.01,
				"cache_creation_price_per_m": 0.00,
				"output_price_per_m": 0.40
			},
			"custom-preview-only": {
				"input_price_per_m": 1.00,
				"cached_price_per_m": 0.10,
				"cache_creation_price_per_m": 0.00,
				"output_price_per_m": 2.00
			}
		}
	}`
	if err := os.WriteFile(pricingPath, []byte(content), 0644); err != nil {
		t.Fatalf("write pricing: %v", err)
	}
	calc, err := calculator.New(pricingPath)
	if err != nil {
		t.Fatalf("new calculator: %v", err)
	}

	t.Run("exact match", func(t *testing.T) {
		cost, err := calc.CalculateCost("claude-sonnet-4-5", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 3.00 {
			t.Errorf("exact match cost = %f, want 3.00", cost)
		}
	})

	t.Run("date-suffix fallback", func(t *testing.T) {
		// claude-sonnet-4-5-20250929 → strips date → claude-sonnet-4-5
		cost, err := calc.CalculateCost("claude-sonnet-4-5-20250929", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 3.00 {
			t.Errorf("date-suffix fallback cost = %f, want 3.00", cost)
		}
	})

	t.Run("thinking-suffix fallback", func(t *testing.T) {
		// claude-sonnet-4-5-thinking → strips -thinking → claude-sonnet-4-5
		cost, err := calc.CalculateCost("claude-sonnet-4-5-thinking", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 3.00 {
			t.Errorf("thinking-suffix fallback cost = %f, want 3.00", cost)
		}
	})

	t.Run("thinking+date combined fallback", func(t *testing.T) {
		// claude-sonnet-4-5-thinking-20251101 → strip date → claude-sonnet-4-5-thinking → strip thinking → claude-sonnet-4-5
		cost, err := calc.CalculateCost("claude-sonnet-4-5-thinking-20251101", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 3.00 {
			t.Errorf("combined fallback cost = %f, want 3.00", cost)
		}
	})

	t.Run("preview-suffix fallback", func(t *testing.T) {
		// gemini-3.1-flash-lite-preview → strips -preview → gemini-3.1-flash-lite
		cost, err := calc.CalculateCost("gemini-3.1-flash-lite-preview", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 0.10 {
			t.Errorf("preview-suffix fallback cost = %f, want 0.10", cost)
		}
	})

	t.Run("progressive fallback to preview entry when date stripped", func(t *testing.T) {
		cost, err := calc.CalculateCost("custom-preview-only-20260501", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 1.00 {
			t.Errorf("progressive fallback cost = %f, want 1.00", cost)
		}
	})

	t.Run("GetModelPrice fallback", func(t *testing.T) {
		price, ok := calc.GetModelPrice("claude-sonnet-4-5-20250929")
		if !ok || price.InputPricePerM != 3.00 {
			t.Errorf("GetModelPrice fallback failed: ok=%v, price=%+v", ok, price)
		}
	})

	t.Run("CalculateCostNoCaching fallback", func(t *testing.T) {
		cost, err := calc.CalculateCostNoCaching("claude-sonnet-4-5-thinking-20251101", 1_000_000, 500_000, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 3.00 {
			t.Errorf("CalculateCostNoCaching fallback cost = %f, want 3.00", cost)
		}
	})

	t.Run("truly unknown model still returns 0", func(t *testing.T) {
		cost, err := calc.CalculateCost("totally-unknown-model", 1_000_000, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 0 {
			t.Errorf("unknown model cost = %f, want 0", cost)
		}
	})
}

func TestCalculatorReplaceBasePricingAndConcurrentRace(t *testing.T) {
	baseJSON := []byte(`{
		"models": {
			"claude-opus-4-7": {"input_price_per_m": 5, "cached_price_per_m": 0.5, "cache_creation_price_per_m": 6.25, "output_price_per_m": 25},
			"gpt-5.5": {"input_price_per_m": 5, "cached_price_per_m": 0.5, "cache_creation_price_per_m": 0, "output_price_per_m": 30}
		}
	}`)
	calc, err := calculator.NewFromBytes(baseJSON)
	if err != nil {
		t.Fatal(err)
	}

	// Custom price override must survive ReplaceBasePricing.
	customPath := filepath.Join(t.TempDir(), "custom_pricing.json")
	if err := os.WriteFile(customPath, []byte(`{
		"claude-opus-4-7": {"input_price_per_m": 99, "cached_price_per_m": 9, "cache_creation_price_per_m": 100, "output_price_per_m": 199}
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := calc.LoadCustomPrices(customPath); err != nil {
		t.Fatal(err)
	}

	// Replacing with identical base pricing reports changed=false.
	changed, err := calc.ReplaceBasePricing(baseJSON)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when base pricing matches")
	}

	// Replacing with updated base pricing reports changed=true and preserves custom overrides.
	updatedBaseJSON := []byte(`{
		"models": {
			"claude-opus-4-7": {"input_price_per_m": 1, "cached_price_per_m": 0.1, "cache_creation_price_per_m": 1, "output_price_per_m": 2},
			"gpt-5.5": {"input_price_per_m": 6, "cached_price_per_m": 0.6, "cache_creation_price_per_m": 0, "output_price_per_m": 36},
			"new-model": {"input_price_per_m": 2, "cached_price_per_m": 0.2, "cache_creation_price_per_m": 0, "output_price_per_m": 8}
		}
	}`)
	changed, err = calc.ReplaceBasePricing(updatedBaseJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true when base pricing changes")
	}

	customPrice, ok := calc.GetModelPrice("claude-opus-4-7")
	if !ok || customPrice.InputPricePerM != 99 {
		t.Fatalf("expected custom override to survive base reload, got ok=%v price=%+v", ok, customPrice)
	}
	newPrice, ok := calc.GetModelPrice("new-model")
	if !ok || newPrice.OutputPricePerM != 8 {
		t.Fatalf("expected new base model to be present, got ok=%v price=%+v", ok, newPrice)
	}

	// Concurrent readers and writers must pass cleanly under -race.
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = calc.CalculateCost("gpt-5.5", 1000, 100, 0, 200)
				_, _ = calc.GetModelPrice("claude-opus-4-7")
				_, _ = calc.CalculateCostNoCaching("new-model", 1000, 100, 0, 200)
				_ = calc.ListModels()
				if id%2 == 0 && i%10 == 0 {
					if i%20 == 0 {
						_, _ = calc.ReplaceBasePricing(baseJSON)
					} else {
						_, _ = calc.ReplaceBasePricing(updatedBaseJSON)
					}
					_ = calc.LoadCustomPrices(customPath)
				}
			}
		}(g)
	}
	wg.Wait()
}
