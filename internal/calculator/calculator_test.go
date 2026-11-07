package calculator_test

import (
	"os"
	"path/filepath"
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
