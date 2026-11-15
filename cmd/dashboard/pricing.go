package main

import (
	"token-ray/internal/calculator"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

//go:embed pricing_table.json
var embeddedPricing []byte

var pricingTableURLs = []string{
	"https://raw.githubusercontent.com/icebear0828/token-ray/main/cmd/dashboard/pricing_table.json",
}

func fetchDynamicPricingFromURLs(urls []string, timeout time.Duration) ([]byte, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no pricing URLs configured")
	}
	var lastErr error
	for _, url := range urls {
		data, err := fetchDynamicPricing(url, timeout)
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
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}
