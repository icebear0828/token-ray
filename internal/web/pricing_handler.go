package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"token-ray/internal/calculator"
	"token-ray/internal/config"
	"token-ray/internal/dashboard"
	"token-ray/internal/db"
)

type PricingEntry struct {
	Model                  string  `json:"model"`
	InputPricePerM         float64 `json:"input_price_per_m"`
	CachedPricePerM        float64 `json:"cached_price_per_m"`
	CacheCreationPricePerM float64 `json:"cache_creation_price_per_m"`
	OutputPricePerM        float64 `json:"output_price_per_m"`
}

func handleGetPricing(w http.ResponseWriter, r *http.Request, calc *calculator.Calculator) {
	models := calc.ListModels()
	sort.Strings(models)
	var entries []PricingEntry
	for _, m := range models {
		price, _ := calc.GetModelPrice(m)
		entries = append(entries, PricingEntry{
			Model:                  m,
			InputPricePerM:         price.InputPricePerM,
			CachedPricePerM:        price.CachedPricePerM,
			CacheCreationPricePerM: price.CacheCreationPricePerM,
			OutputPricePerM:        price.OutputPricePerM,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func handlePutPricing(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator, statsCache *dashboard.StatsCache) {
	var entries []PricingEntry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	customPrices := make(map[string]calculator.ModelPrice)
	for _, e := range entries {
		if e.InputPricePerM < 0 {
			e.InputPricePerM = 0
		}
		if e.CachedPricePerM < 0 {
			e.CachedPricePerM = 0
		}
		if e.CacheCreationPricePerM < 0 {
			e.CacheCreationPricePerM = 0
		}
		if e.OutputPricePerM < 0 {
			e.OutputPricePerM = 0
		}

		customPrices[e.Model] = calculator.ModelPrice{
			InputPricePerM:         e.InputPricePerM,
			CachedPricePerM:        e.CachedPricePerM,
			CacheCreationPricePerM: e.CacheCreationPricePerM,
			OutputPricePerM:        e.OutputPricePerM,
		}
	}

	existingCustomPrices := make(map[string]calculator.ModelPrice)
	customPricingPath := config.GetCustomPricingPath()
	if data, err := os.ReadFile(customPricingPath); err == nil {
		if err := json.Unmarshal(data, &existingCustomPrices); err != nil {
			http.Error(w, "Existing custom_pricing.json is corrupted. Refusing to overwrite.", http.StatusInternalServerError)
			return
		}
	}

	for k, v := range customPrices {
		existingCustomPrices[k] = v
	}

	data, err := json.MarshalIndent(existingCustomPrices, "", "  ")
	if err != nil {
		http.Error(w, "Failed to marshal pricing data", http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(filepath.Dir(customPricingPath), 0755); err != nil {
		http.Error(w, "Failed to prepare pricing directory", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(customPricingPath, data, 0644); err != nil {
		http.Error(w, "Failed to write pricing data to disk", http.StatusInternalServerError)
		return
	}

	calc.UpdatePrices(customPrices)
	if _, err := database.RecalculateUsageCosts(calc.CalculateCost); err != nil {
		http.Error(w, "Failed to recalculate usage costs", http.StatusInternalServerError)
		return
	}
	statsCache.Clear()
	w.WriteHeader(http.StatusOK)
}
