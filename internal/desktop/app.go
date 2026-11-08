package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/updater"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application binding layer.
// It exposes Go backend functionality to the frontend via Wails bindings.
type App struct {
	ctx        context.Context
	database   *db.DB
	calc       *calculator.Calculator
	statsCache *dashboard.StatsCache
	mu         sync.Mutex
	pendingShow bool
}

// NewApp creates a new App with references to the shared backend services.
func NewApp(database *db.DB, calc *calculator.Calculator) *App {
	return &App{
		database:   database,
		calc:       calc,
		statsCache: dashboard.NewStatsCache(2 * time.Second),
	}
}

// Startup is called by Wails when the application starts.
func (a *App) Startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	pending := a.pendingShow
	a.pendingShow = false
	a.mu.Unlock()
	if pending {
		a.ShowWindow()
	}
}

// GetContext returns the Wails runtime context.
// NOTE: Returns nil before Startup() is called by Wails. Menu and tray
// callbacks can run before the window exists, so callers must handle nil.
func (a *App) GetContext() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx
}

// --- Stats API (shared types from internal/model/stats.go) ---

// GetStats returns the full dashboard statistics, optionally filtered by device and source.
func (a *App) GetStats(deviceID string, source string) (*model.StatsResponse, error) {
	return a.statsCache.Get(dashboard.StatsCacheKey{
		DeviceID: deviceID,
		Source:   source,
		IsPaused: false,
	}, func() (*model.StatsResponse, error) {
		return dashboard.BuildStats(a.database, a.calc, deviceID, source, false)
	})
}

// --- Cache Savings ---

// GetCacheSavings returns cache savings analysis.
func (a *App) GetCacheSavings(deviceID string) (*model.CacheSavingsResponse, error) {
	if deviceID == "all" {
		deviceID = ""
	}

	records, err := a.database.QueryUsageRecords(time.Time{}, deviceID)
	if err != nil {
		return nil, err
	}

	var actualTotal, hypoTotal float64
	var totalInput, totalCached int

	for _, rec := range records {
		actual, _ := a.calc.CalculateCost(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		hypo, _ := a.calc.CalculateCostNoCaching(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		actualTotal += actual
		hypoTotal += hypo
		totalInput += rec.InputTokens
		totalCached += rec.CachedTokens
	}

	saved := hypoTotal - actualTotal
	savedPct := 0.0
	if hypoTotal > 0 {
		savedPct = (saved / hypoTotal) * 100
	}
	hitRate := 0.0
	hitRate = model.CacheHitRatePercent(totalInput, totalCached)

	return &model.CacheSavingsResponse{
		ActualCost:       actualTotal,
		HypotheticalCost: hypoTotal,
		Saved:            saved,
		SavedPercent:     savedPct,
		CacheHitRate:     hitRate,
	}, nil
}

// --- Device Alias ---

// SetDeviceAlias sets a display name for a device.
func (a *App) SetDeviceAlias(deviceID, displayName string) error {
	return a.database.SetDeviceAlias(deviceID, displayName)
}

// --- Pricing Management (Phase 2) ---

type PricingEntry struct {
	Model           string  `json:"model"`
	InputPricePerM  float64 `json:"input_price_per_m"`
	CachedPricePerM float64 `json:"cached_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
}

// GetPricing returns the current pricing table.
func (a *App) GetPricing() ([]PricingEntry, error) {
	models := a.calc.ListModels()
	var entries []PricingEntry
	for _, m := range models {
		price, _ := a.calc.GetModelPrice(m)
		entries = append(entries, PricingEntry{
			Model:           m,
			InputPricePerM:  price.InputPricePerM,
			CachedPricePerM: price.CachedPricePerM,
			OutputPricePerM: price.OutputPricePerM,
		})
	}
	return entries, nil
}

// --- Window Controls ---

// MinimiseToTray hides the window (tray keeps running).
func (a *App) MinimiseToTray() {
	if ctx := a.GetContext(); ctx != nil {
		runtime.WindowHide(ctx)
	}
}

// ShowWindow restores, unminimises, and focuses the dashboard window. If the
// window is not ready yet (tray/second-instance callbacks can race startup),
// the request is replayed once Startup runs.
func (a *App) ShowWindow() {
	a.mu.Lock()
	ctx := a.ctx
	if ctx == nil {
		a.pendingShow = true
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
	runtime.WindowSetAlwaysOnTop(ctx, false)
}

// Quit exits the application.
func (a *App) Quit() {
	if ctx := a.GetContext(); ctx != nil {
		runtime.Quit(ctx)
	}
}

// --- Config ---

// GetConfig returns the current app configuration.
func (a *App) GetConfig() (*config.AppConfig, error) {
	return config.LoadConfig()
}

// SaveConfig persists the app configuration.
func (a *App) SaveConfig(cfg *config.AppConfig) error {
	return config.SaveConfig(cfg)
}

// --- System ---

// OpenSystemLogs opens the logs directory in the native file explorer.
func (a *App) OpenSystemLogs() {
	ctx := a.GetContext()
	if ctx == nil {
		return
	}
	logPath := filepath.Join(config.GetDataDir(), "stats")
	// Wails BrowserOpenURL can open local directories if prefixed with file://
	runtime.BrowserOpenURL(ctx, "file://"+logPath)
}

// --- Updater ---

// CheckForUpdates checks for a new release
func (a *App) CheckForUpdates(currentVersion string) (*updater.Release, error) {
	token := os.Getenv("DASHBOARD_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	return updater.CheckForUpdates(currentVersion, token)
}

// ApplyUpdate attempts to apply the OTA update
func (a *App) ApplyUpdate() error {
	// 2. Get token from env
	token := os.Getenv("DASHBOARD_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	// 1. Fetch latest release
	// Hardcoding "0.0.0" as current version since we just want the latest release info
	release, err := updater.CheckForUpdates("0.0.0", token)
	if err != nil {
		return fmt.Errorf("failed to get release info: %v", err)
	}
	if release == nil {
		return fmt.Errorf("no release found")
	}

	// 3. Apply update
	err = updater.ApplyUpdate(release, token)
	if err != nil {
		return err
	}

	return nil
}
