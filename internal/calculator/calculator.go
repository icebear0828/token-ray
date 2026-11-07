package calculator

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// dateSuffixRe matches trailing date suffixes like -20250929, -20251101.
var dateSuffixRe = regexp.MustCompile(`-\d{8}$`)

// NormalizeModelName strips known variant suffixes from a model name so that
// unknown variants fall back to their base model's pricing.
//
// Rules applied in order:
//  1. Strip trailing date suffix  (e.g. -20250929)
//  2. Strip trailing -thinking suffix
//  3. Strip trailing -preview suffix
//
// The function returns the stripped name. If no stripping applies the input is
// returned unchanged.
func NormalizeModelName(model string) string {
	// Strip date suffix first (may be combined with -thinking, e.g. -thinking-20251001)
	model = dateSuffixRe.ReplaceAllString(model, "")
	// Strip -thinking
	model = strings.TrimSuffix(model, "-thinking")
	// Strip -preview
	model = strings.TrimSuffix(model, "-preview")
	return model
}

type ModelPrice struct {
	InputPricePerM         float64 `json:"input_price_per_m"`
	CachedPricePerM        float64 `json:"cached_price_per_m"`
	CacheCreationPricePerM float64 `json:"cache_creation_price_per_m"`
	OutputPricePerM        float64 `json:"output_price_per_m"`
}

type PricingTable struct {
	Models map[string]ModelPrice `json:"models"`
}

type Calculator struct {
	mu     sync.RWMutex
	prices PricingTable
}

// NewFromBytes creates a Calculator from raw JSON pricing data.
func NewFromBytes(data []byte) (*Calculator, error) {
	var pt PricingTable
	if err := json.Unmarshal(data, &pt); err != nil {
		return nil, fmt.Errorf("unmarshal pricing table: %w", err)
	}
	if pt.Models == nil {
		pt.Models = make(map[string]ModelPrice)
	}
	return &Calculator{prices: pt}, nil
}

// New creates a Calculator by reading a pricing JSON file from disk.
func New(pricingFilePath string) (*Calculator, error) {
	data, err := os.ReadFile(pricingFilePath)
	if err != nil {
		return nil, fmt.Errorf("read pricing file: %w", err)
	}
	return NewFromBytes(data)
}

// UpdatePrices merges custom pricing into the calculator.
func (c *Calculator) UpdatePrices(customPrices map[string]ModelPrice) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range customPrices {
		c.prices.Models[k] = v
	}
}

// LoadCustomPrices reads a JSON file containing a map of ModelPrice and merges it.
func (c *Calculator) LoadCustomPrices(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // silently ignore if no custom pricing exists
		}
		return err
	}
	var custom map[string]ModelPrice
	if err := json.Unmarshal(data, &custom); err != nil {
		return err
	}
	c.UpdatePrices(custom)
	return nil
}

// lookupPrice returns the ModelPrice for model, applying fallback normalization
// when an exact match is not found in the pricing table. The caller must NOT
// hold c.mu when calling this function.
func (c *Calculator) lookupPrice(model string) (ModelPrice, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if p, ok := c.prices.Models[model]; ok {
		return p, true
	}

	// Try progressively normalized forms until a match is found.
	normalized := NormalizeModelName(model)
	if normalized != model {
		if p, ok := c.prices.Models[normalized]; ok {
			return p, true
		}
	}
	return ModelPrice{}, false
}

// CalculateCost takes the specific token metrics and returns the estimated USD cost.
// When the exact model name is not found in the pricing table, it falls back to a
// normalized base name (stripping date/thinking/preview suffixes).
func (c *Calculator) CalculateCost(model string, inputTokens, cachedTokens, cacheCreationTokens, outputTokens int) (float64, error) {
	price, ok := c.lookupPrice(model)
	if !ok {
		return 0, nil // unknown model, skip silently
	}
	
	baseInput := inputTokens - cachedTokens - cacheCreationTokens
	if baseInput < 0 {
		baseInput = 0
	}
	
	inputCost := (float64(baseInput) / 1_000_000.0) * price.InputPricePerM
	cachedCost := (float64(cachedTokens) / 1_000_000.0) * price.CachedPricePerM
	cacheCreationCost := (float64(cacheCreationTokens) / 1_000_000.0) * price.CacheCreationPricePerM
	outputCost := (float64(outputTokens) / 1_000_000.0) * price.OutputPricePerM
	
	return inputCost + cachedCost + cacheCreationCost + outputCost, nil
}

// GetModelPrice returns the price table for a specific model, applying fallback
// normalization when an exact match is not found.
func (c *Calculator) GetModelPrice(model string) (ModelPrice, bool) {
	return c.lookupPrice(model)
}

// ListModels returns all model names in the pricing table.
func (c *Calculator) ListModels() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	models := make([]string, 0, len(c.prices.Models))
	for m := range c.prices.Models {
		models = append(models, m)
	}
	return models
}

// CalculateCostNoCaching computes a hypothetical cost where cached tokens are
// charged at the full input price instead of the discounted cached price.
// Falls back to normalized model name when exact match is not found.
func (c *Calculator) CalculateCostNoCaching(model string, inputTokens, cachedTokens, cacheCreationTokens, outputTokens int) (float64, error) {
	price, ok := c.lookupPrice(model)
	if !ok {
		return 0, nil
	}
	// Treat ALL input tokens (including cached and cache_creation) at full input price
	inputCost := (float64(inputTokens) / 1_000_000.0) * price.InputPricePerM
	outputCost := (float64(outputTokens) / 1_000_000.0) * price.OutputPricePerM
	return inputCost + outputCost, nil
}
