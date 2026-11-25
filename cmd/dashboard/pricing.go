package main

import (
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

//go:embed pricing_table.json
var embeddedPricing []byte

var pricingTableURLs = []string{
	"https://raw.githubusercontent.com/icebear0828/token-ray/main/cmd/dashboard/pricing_table.json",
}

func fetchDynamicPricingFromURLs(urls []string, timeout time.Duration) ([]byte, error) {
	return fetchDynamicPricingFromURLsWithContext(context.Background(), urls, timeout)
}

func fetchDynamicPricingFromURLsWithContext(ctx context.Context, urls []string, timeout time.Duration) ([]byte, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no pricing URLs configured")
	}
	var lastErr error
	for _, url := range urls {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		data, err := fetchDynamicPricingWithContext(ctx, url, timeout)
		if err == nil {
			return data, nil
		}
		lastErr = fmt.Errorf("%s: %w", url, err)
	}
	return nil, lastErr
}

func mergePricingData(baseData []byte, overrideData []byte) ([]byte, error) {
	var base calculator.PricingTable
	if err := json.Unmarshal(baseData, &base); err != nil {
		return nil, fmt.Errorf("unmarshal base pricing table: %w", err)
	}
	if base.Models == nil {
		base.Models = make(map[string]calculator.ModelPrice)
	}

	var override calculator.PricingTable
	if err := json.Unmarshal(overrideData, &override); err != nil {
		return nil, fmt.Errorf("unmarshal override pricing table: %w", err)
	}
	for model, price := range override.Models {
		base.Models[model] = price
	}

	merged, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshal merged pricing table: %w", err)
	}
	return merged, nil
}

func fetchDynamicPricing(url string, timeout time.Duration) ([]byte, error) {
	return fetchDynamicPricingWithContext(context.Background(), url, timeout)
}

func fetchDynamicPricingWithContext(ctx context.Context, url string, timeout time.Duration) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}

// syncDynamicPricingAndRecalc fetches dynamic pricing asynchronously and only
// recalculates persisted usage costs when effective prices changed or $0 rows
// exist. If anything changed, statsCache is invalidated so subsequent HTTP reads
// observe updated costs and unit prices.
func syncDynamicPricingAndRecalc(ctx context.Context, database *db.DB, calc *calculator.Calculator, statsCache *dashboard.StatsCache, urls []string, timeout time.Duration) {
	var pricesChanged bool
	if dynPricing, err := fetchDynamicPricingFromURLsWithContext(ctx, urls, timeout); err == nil {
		if mergedPricing, err := mergePricingData(embeddedPricing, dynPricing); err == nil {
			changed, err := calc.ReplaceBasePricing(mergedPricing)
			if err != nil {
				log.Printf("Failed to apply dynamic pricing table: %v", err)
			} else {
				pricesChanged = changed
				fmt.Println("☁️  Successfully synced dynamic pricing table from GitHub.")
			}
		} else {
			log.Printf("Failed to merge dynamic pricing table, keeping embedded version: %v", err)
		}
	} else if ctx.Err() == nil {
		log.Printf("Dynamic pricing fetch unavailable, keeping embedded version: %v", err)
	}

	if ctx.Err() != nil {
		return
	}

	needsRecalc := pricesChanged
	if !needsRecalc {
		hasZero, err := database.HasZeroCostUsageRecords()
		if err != nil {
			log.Printf("Failed to check zero-cost usage records: %v", err)
			return
		}
		needsRecalc = hasZero
	}

	var recalculated int64
	if needsRecalc && ctx.Err() == nil {
		n, err := database.RecalculateUsageCosts(calc.CalculateCost)
		if err != nil {
			log.Printf("Failed to recalculate usage costs: %v", err)
		} else if n > 0 {
			recalculated = n
			fmt.Printf("💸 Recalculated usage costs for %d existing records.\n", n)
		}
	}

	if (pricesChanged || recalculated > 0) && statsCache != nil {
		statsCache.Clear()
	}
}
