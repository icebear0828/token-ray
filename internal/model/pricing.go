package model

// PricingEntry is a single row of the editable model pricing table. It is the
// wire shape shared by the HTTP API and the Wails desktop binding.
type PricingEntry struct {
	Model                  string  `json:"model"`
	InputPricePerM         float64 `json:"input_price_per_m"`
	CachedPricePerM        float64 `json:"cached_price_per_m"`
	CacheCreationPricePerM float64 `json:"cache_creation_price_per_m"`
	OutputPricePerM        float64 `json:"output_price_per_m"`
}
