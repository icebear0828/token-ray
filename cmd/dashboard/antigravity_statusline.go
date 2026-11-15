package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"token-ray/internal/antigravity"
	"token-ray/internal/calculator"
	"token-ray/internal/config"
	"token-ray/internal/db"
)

const maxStatuslinePayloadBytes = 1024 * 1024

func runAntigravityStatuslineCommand(deviceID string) int {
	database, err := openStatuslineDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Token Ray: failed to open database: %v\n", err)
		return 1
	}
	defer database.Close()

	calc, err := calculator.NewFromBytes(embeddedPricing)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Token Ray: failed to initialize pricing: %v\n", err)
		return 1
	}
	if err := calc.LoadCustomPrices(config.GetCustomPricingPath()); err != nil {
		fmt.Fprintf(os.Stderr, "Token Ray: failed to load custom pricing: %v\n", err)
	}

	return runAntigravityStatusline(database, calc, deviceID, os.Stdin, os.Stdout, os.Stderr)
}

func openStatuslineDB() (*db.DB, error) {
	appDataDir := config.GetDataDir()
	dbPath := filepath.Join(appDataDir, "stats", "usage.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	return db.New(dbPath)
}

func runAntigravityStatusline(database *db.DB, calc *calculator.Calculator, deviceID string, in io.Reader, out io.Writer, errOut io.Writer) int {
	raw, err := readOneStatuslinePayload(in)
	if err != nil {
		fmt.Fprintf(errOut, "Token Ray: failed to read Antigravity statusline payload: %v\n", err)
		return 1
	}

	usage, ok, err := antigravity.ParseStatusline(raw, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(errOut, "Token Ray: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintln(out, "Token Ray: no Antigravity usage")
		return 0
	}

	cost, err := calc.CalculateCost(usage.Model, usage.InputTokens, usage.CachedTokens, usage.CacheCreationTokens, usage.OutputTokens)
	if err != nil {
		fmt.Fprintf(errOut, "Token Ray: failed to calculate Antigravity cost: %v\n", err)
		return 1
	}
	if err := database.InsertUsage(usage, cost, deviceID); err != nil {
		fmt.Fprintf(errOut, "Token Ray: failed to write Antigravity usage: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Token Ray: Antigravity %s tok $%.4f\n", formatStatuslineTokens(antigravity.TotalTokens(usage)), cost)
	return 0
}

func readOneStatuslinePayload(in io.Reader) ([]byte, error) {
	limited := io.LimitReader(in, maxStatuslinePayloadBytes+1)
	decoder := json.NewDecoder(limited)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if len(raw) > maxStatuslinePayloadBytes {
		return nil, fmt.Errorf("payload exceeds %d bytes", maxStatuslinePayloadBytes)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	return raw, nil
}

func formatStatuslineTokens(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000*1000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/(1000*1000))
}
