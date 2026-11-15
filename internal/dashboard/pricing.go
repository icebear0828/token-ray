package dashboard

import (
	"sort"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/model"
)

// BuildPricingEntries returns the pricing table sorted by model name.
func BuildPricingEntries(calc *calculator.Calculator) []model.PricingEntry {
	models := calc.ListModels()
	sort.Strings(models)

	entries := make([]model.PricingEntry, 0, len(models))
	for _, m := range models {
		price, _ := calc.GetModelPrice(m)
		entries = append(entries, model.PricingEntry{
			Model:                  m,
			InputPricePerM:         price.InputPricePerM,
			CachedPricePerM:        price.CachedPricePerM,
			CacheCreationPricePerM: price.CacheCreationPricePerM,
			OutputPricePerM:        price.OutputPricePerM,
		})
	}
	return entries
}
