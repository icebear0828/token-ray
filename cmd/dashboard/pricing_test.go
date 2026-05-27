package main

import (
	"ai-flight-dashboard/internal/calculator"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedPricingMatchesOfficialRates(t *testing.T) {
	// Sources checked on 2026-05-19:
	// Anthropic: https://platform.claude.com/docs/en/docs/about-claude/models/overview
	// Gemini: https://ai.google.dev/gemini-api/docs/pricing
	// OpenAI: https://developers.openai.com/api/docs/models/gpt-5.5
	var table calculator.PricingTable
	if err := json.Unmarshal(embeddedPricing, &table); err != nil {
		t.Fatalf("unmarshal embedded pricing: %v", err)
	}

	expected := map[string]calculator.ModelPrice{
		"gemini-2.5-pro": {
			InputPricePerM:  1.25,
			CachedPricePerM: 0.125,
			OutputPricePerM: 10.00,
		},
		"gemini-2.5-flash": {
			InputPricePerM:  0.30,
			CachedPricePerM: 0.03,
			OutputPricePerM: 2.50,
		},
		"gemini-3-flash-preview": {
			InputPricePerM:  0.50,
			CachedPricePerM: 0.05,
			OutputPricePerM: 3.00,
		},
		"gemini-3-flash": {
			InputPricePerM:  0.50,
			CachedPricePerM: 0.05,
			OutputPricePerM: 3.00,
		},
		"gemini-3.5-flash": {
			InputPricePerM:         1.50,
			CachedPricePerM:        0.15,
			CacheCreationPricePerM: 1.50,
			OutputPricePerM:        9.00,
		},
		"gemini-3.1-pro-preview": {
			InputPricePerM:  2.00,
			CachedPricePerM: 0.20,
			OutputPricePerM: 12.00,
		},
		"gpt-5.5": {
			InputPricePerM:  5.00,
			CachedPricePerM: 0.50,
			OutputPricePerM: 30.00,
		},
		"gpt-5.4": {
			InputPricePerM:  2.50,
			CachedPricePerM: 0.25,
			OutputPricePerM: 15.00,
		},
		"gpt-5.4-mini": {
			InputPricePerM:  0.75,
			CachedPricePerM: 0.075,
			OutputPricePerM: 4.50,
		},
		"claude-sonnet-4-6": {
			InputPricePerM:         3.00,
			CachedPricePerM:        0.30,
			CacheCreationPricePerM: 3.75,
			OutputPricePerM:        15.00,
		},
		"claude-opus-4-7": {
			InputPricePerM:         5.00,
			CachedPricePerM:        0.50,
			CacheCreationPricePerM: 6.25,
			OutputPricePerM:        25.00,
		},
		"claude-opus-4-6": {
			InputPricePerM:         5.00,
			CachedPricePerM:        0.50,
			CacheCreationPricePerM: 6.25,
			OutputPricePerM:        25.00,
		},
		"claude-haiku-4-5-20251001": {
			InputPricePerM:         1.00,
			CachedPricePerM:        0.10,
			CacheCreationPricePerM: 1.25,
			OutputPricePerM:        5.00,
		},
	}

	for model, want := range expected {
		got, ok := table.Models[model]
		if !ok {
			t.Fatalf("missing pricing for %s", model)
		}
		if got != want {
			t.Fatalf("pricing for %s = %+v, want %+v", model, got, want)
		}
	}
}

func TestFetchDynamicPricingFromURLsFallsBackAfterFailure(t *testing.T) {
	goodJSON := `{"models":{"test-model":{"input_price_per_m":1,"cached_price_per_m":0,"cache_creation_price_per_m":0,"output_price_per_m":2}}}`
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path == "/first.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(goodJSON))
	}))
	defer server.Close()

	got, err := fetchDynamicPricingFromURLs([]string{server.URL + "/first.json", server.URL + "/second.json"}, time.Second)
	if err != nil {
		t.Fatalf("expected fallback fetch to succeed: %v", err)
	}
	if string(got) != goodJSON {
		t.Fatalf("unexpected pricing payload: %s", got)
	}
	if strings.Join(requests, ",") != "/first.json,/second.json" {
		t.Fatalf("unexpected request sequence: %v", requests)
	}
}

func TestMergePricingDataPreservesEmbeddedModelsWhenDynamicIsStale(t *testing.T) {
	baseJSON := []byte(`{
		"models": {
			"gemini-3.5-flash": {
				"input_price_per_m": 1.5,
				"cached_price_per_m": 0.15,
				"cache_creation_price_per_m": 1.5,
				"output_price_per_m": 9
			},
			"gemini-2.5-pro": {
				"input_price_per_m": 1.25,
				"cached_price_per_m": 0.125,
				"cache_creation_price_per_m": 0,
				"output_price_per_m": 10
			}
		}
	}`)
	dynamicJSON := []byte(`{
		"models": {
			"gemini-2.5-pro": {
				"input_price_per_m": 2,
				"cached_price_per_m": 0.2,
				"cache_creation_price_per_m": 0,
				"output_price_per_m": 12
			}
		}
	}`)

	merged, err := mergePricingData(baseJSON, dynamicJSON)
	if err != nil {
		t.Fatalf("merge pricing data: %v", err)
	}

	var table calculator.PricingTable
	if err := json.Unmarshal(merged, &table); err != nil {
		t.Fatalf("unmarshal merged pricing: %v", err)
	}
	if got, ok := table.Models["gemini-3.5-flash"]; !ok {
		t.Fatal("expected embedded-only gemini-3.5-flash pricing to be preserved")
	} else if got.InputPricePerM != 1.5 || got.CacheCreationPricePerM != 1.5 || got.OutputPricePerM != 9 {
		t.Fatalf("unexpected preserved pricing: %+v", got)
	}
	if got := table.Models["gemini-2.5-pro"]; got.InputPricePerM != 2 || got.OutputPricePerM != 12 {
		t.Fatalf("expected dynamic pricing to override embedded model, got %+v", got)
	}
}

func TestDynamicPricingURLsUseCurrentRepository(t *testing.T) {
	if len(pricingTableURLs) == 0 {
		t.Fatal("expected at least one dynamic pricing URL")
	}
	for _, url := range pricingTableURLs {
		if strings.Contains(url, "githubusercontent.com/icebear0828/ai-flight-dashboard/") ||
			strings.Contains(url, "github.com/icebear0828/ai-flight-dashboard/") {
			t.Fatalf("dynamic pricing URL still points to retired repository: %s", url)
		}
		if !strings.Contains(url, "githubusercontent.com/icebear0828/token-ray/") &&
			!strings.Contains(url, "github.com/icebear0828/token-ray/") {
			t.Fatalf("dynamic pricing URL should use current repository: %s", url)
		}
	}
}
