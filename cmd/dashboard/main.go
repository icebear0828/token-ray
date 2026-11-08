package main

import (
	root "ai-flight-dashboard"
	"ai-flight-dashboard/internal/alert"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/desktop"
	"ai-flight-dashboard/internal/forwarder"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/plugin"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/sources"
	"ai-flight-dashboard/internal/tui"
	"ai-flight-dashboard/internal/updater"
	"ai-flight-dashboard/internal/watcher"
	"ai-flight-dashboard/internal/web"
	"context"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	wailsrun "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse CLI flags
	webMode := flag.Bool("web", false, "Run in web dashboard mode")
	tuiMode := flag.Bool("tui", false, "Run in legacy Terminal UI mode")
	port := flag.String("port", "19100", "HTTP port for web mode")
	forwardTo := flag.String("forward-to", "", "Forward local usage to remote dashboard URL (e.g. http://server:19100/api/track)")
	token := flag.String("token", "", "Authorization token for web mode or forwarder")
	defaultDevice, _ := os.Hostname()
	if defaultDevice == "" {
		defaultDevice = "local"
	}
	deviceID := flag.String("device-id", defaultDevice, "Device identifier")
	billingModeStr := flag.String("billing-mode", "auto", "Billing mode: auto, subscription, api")
	plan := flag.String("plan", "", "Subscription plan: pro, max5, max20 (only for subscription mode)")
	budgetDaily := flag.Float64("budget-daily", 0, "Daily budget limit in USD (only for api mode, 0=disabled)")
	syncMode := flag.String("sync-mode", "poll", "Sync mode: poll (default), fsnotify, once")
	lanMode := flag.Bool("lan", true, "Enable UDP Multicast LAN discovery and broadcast")
	dataDir := flag.String("data-dir", "", "Data directory for database and config (default: ~/.ai-flight-dashboard; env: AI_FLIGHT_DASHBOARD_DATA_DIR)")
	flag.BoolVar(webMode, "w", false, "Run in web dashboard mode (shorthand)")
	flag.StringVar(port, "p", "19100", "HTTP port for web mode (shorthand)")
	flag.Parse()
	args := flag.Args()
	repairHistory := len(args) > 0 && args[0] == "repair-history"
	if *dataDir != "" {
		config.SetDataDir(*dataDir)
	}

	// Default to GUI mode unless another mode is explicitly requested
	runGui := true
	if *tuiMode || *webMode || *forwardTo != "" || len(flag.Args()) > 0 {
		runGui = false
	}

	// Read token from environment variable if not provided via flag
	if *token == "" {
		*token = os.Getenv("DASHBOARD_TOKEN")
	}

	// Validate billing mode
	billingMode, err := model.ParseBillingMode(*billingModeStr)
	if err != nil {
		log.Fatalf("Invalid billing mode: %v", err)
	}
	if billingMode == model.BillingSubscription && *plan != "" {
		if _, ok := model.PlanLimits[*plan]; !ok {
			log.Fatalf("Unknown plan %q: must be pro, max5, or max20", *plan)
		}
	}

	// Subcommand dispatch: export | import | dedup | repair-history | antigravity-statusline
	if len(args) > 0 {
		switch args[0] {
		case "antigravity-statusline":
			os.Exit(runAntigravityStatuslineCommand(*deviceID))
		case "export":
			runExport(*deviceID)
			return
		case "import":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "Usage: dashboard import <file.csv>")
				os.Exit(1)
			}
			runImport(args[1])
			return
		case "dedup":
			runDedup()
			return
		case "repair-history":
			// Handled after calculator/database initialization.
		}
	}

	// Forwarder Mode
	if *forwardTo != "" {
		fmt.Printf("🚀 Starting forwarder mode. Device: %s, Target: %s\n", *deviceID, *forwardTo)

		w, err := watcher.New(*deviceID)
		if err != nil {
			log.Fatalf("Failed to initialize watcher: %v", err)
		}
		defer w.Close()

		home, _ := os.UserHomeDir()
		claudeProjects := filepath.Join(home, ".claude", "projects")
		if _, err := os.Stat(claudeProjects); err == nil {
			w.WatchDirRecursive(claudeProjects)
		}
		geminiTmp := filepath.Join(home, ".gemini", "tmp")
		if _, err := os.Stat(geminiTmp); err == nil {
			w.WatchDirRecursive(geminiTmp)
		}

		fw := forwarder.New(*forwardTo, *token, *deviceID)
		fw.Start(w.UsageChan) // Blocks forever
		return
	}

	// Initialize Calculator from pricing table
	pricingData := embeddedPricing
	if dynPricing, err := fetchDynamicPricingFromURLs(pricingTableURLs, 3*time.Second); err == nil {
		if mergedPricing, err := mergePricingData(embeddedPricing, dynPricing); err == nil {
			fmt.Println("☁️  Successfully fetched dynamic pricing table from GitHub.")
			pricingData = mergedPricing
		} else {
			fmt.Printf("⚠️  Failed to merge dynamic pricing table, using embedded version: %v\n", err)
		}
	} else {
		fmt.Printf("⚠️  Failed to fetch dynamic pricing table, using embedded version: %v\n", err)
	}

	calc, err := calculator.NewFromBytes(pricingData)
	if err != nil {
		log.Fatalf("Failed to initialize calculator: %v", err)
	}

	if err := calc.LoadCustomPrices(config.GetCustomPricingPath()); err != nil {
		fmt.Printf("⚠️  Failed to load custom pricing: %v\n", err)
	}
	home, _ := os.UserHomeDir()

	// Initialize Database
	appDataDir := config.GetDataDir()
	statsDir := filepath.Join(appDataDir, "stats")
	os.MkdirAll(statsDir, 0755)

	// GUI builds have no console; capture startup errors (including lock
	// conflicts) in a file so "won't open" is diagnosable.
	if runGui {
		if f, err := os.OpenFile(filepath.Join(statsDir, "gui.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			log.SetOutput(io.MultiWriter(os.Stderr, f))
			defer f.Close()
		}
	}

	processLock := acquireProcessLock(appDataDir, runGui)
	defer processLock.Release()

	dbPath := filepath.Join(statsDir, "usage.db")
	fmt.Printf("📁 Data dir: %s\n", appDataDir)
	fmt.Printf("🧾 Config: %s\n", config.GetConfigPath())
	fmt.Printf("💾 Database: %s\n", dbPath)
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	if n, err := database.BackfillProjectsFromFilePaths(watcher.ExtractProjectName); err != nil {
		log.Printf("Failed to backfill project names: %v", err)
	} else if n > 0 {
		fmt.Printf("🧭 Backfilled project names for %d existing records.\n", n)
	}
	if n, err := database.RecalculateUsageCosts(calc.CalculateCost); err != nil {
		log.Printf("Failed to recalculate usage costs: %v", err)
	} else if n > 0 {
		fmt.Printf("💸 Recalculated usage costs for %d existing records.\n", n)
	}
	const projectMetadataMigrationKey = "migration:project-metadata-v2"
	if done, err := database.GetOffset(projectMetadataMigrationKey); err == nil && done == 0 {
		var queued int64
		for _, pattern := range []string{
			"%/.claude/projects/%",
			"%\\.claude\\projects\\%",
			"%/.gemini/tmp/%",
			"%\\.gemini\\tmp\\%",
		} {
			n, err := database.ResetOffsetsLike(pattern)
			if err != nil {
				log.Printf("Failed to reset project metadata scan offsets for %q: %v", pattern, err)
				continue
			}
			queued += n
		}
		if queued > 0 {
			fmt.Printf("🧭 Queued %d log files for project metadata backfill.\n", queued)
		}
		if err := database.SetOffset(projectMetadataMigrationKey, 1); err != nil {
			log.Printf("Failed to mark project metadata migration complete: %v", err)
		}
	}
	queueCodexTelemetryBackfill(database)
	// Collect built-in scan directories for the repair flow.
	var scanDirs []string

	claudeProjects := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(claudeProjects); err == nil {
		scanDirs = append(scanDirs, claudeProjects)
	}

	geminiTmp := filepath.Join(home, ".gemini", "tmp")
	if _, err := os.Stat(geminiTmp); err == nil {
		scanDirs = append(scanDirs, geminiTmp)
	}

	appConfig, _ := config.LoadConfig()
	for _, dir := range appConfig.ExtraWatchDirs {
		if _, err := os.Stat(dir); err == nil {
			scanDirs = append(scanDirs, dir)
		}
	}
	if repairHistory {
		runRepairHistory(database, calc, *deviceID, scanDirs)
		return
	}

	// Initialize Watcher immediately (instant, ~26µs)
	w, err := watcher.New(*deviceID)
	if err != nil {
		log.Fatalf("Failed to initialize watcher: %v", err)
	}
	defer w.Close()

	var lanInst *lan.LAN
	lanInst = newLANInstance(*lanMode, appConfig.EnableLAN, *token, *deviceID, *port)
	if lanInst == nil {
		fmt.Println("📡 LAN discovery is disabled in settings.")
	} else {
		if *token == "" {
			fmt.Println("📡 LAN discovery and private-network sync are enabled by default.")
		}
	}

	var lanController *runtimeLANController

	// Build the source registry: built-ins first, then enabled plugins.
	registry := source.NewRegistry()
	registry.Register(sources.NewClaudeSource(home))
	registry.Register(sources.NewGeminiSource(home))
	registry.Register(sources.NewExtraDirsSource())
	registry.Register(sources.NewCodexSource())
	registry.Register(sources.NewAntigravitySource())
	registry.Register(sources.NewDSHSource(home))
	registry.Register(sources.NewPiSource(home))

	pluginManager := plugin.NewManager(config.GetDataDir(), updater.CurrentVersion, func(id string) bool {
		cfg, err := config.LoadConfig()
		if err != nil {
			return true
		}
		for _, disabled := range cfg.DisabledPlugins {
			if disabled == id {
				return false
			}
		}
		return true
	})
	for _, src := range pluginManager.LoadSources() {
		registry.Register(src)
	}

	sink := source.NewDBSink(database, calc, *deviceID, func(u model.TokenUsage) {
		select {
		case w.UsageChan <- u:
		default:
		}
		select {
		case w.BroadcastChan <- u:
		default:
		}
	})
	pluginController := plugin.NewController(pluginManager, registry, w, sink)

	// Route filesystem events to the owning source instead of parsing here.
	w.SetFileHandler(func(path string) {
		src := registry.SourceForPath(path)
		if src == nil {
			return
		}
		fileScanner, ok := src.(source.FileScanner)
		if !ok {
			return
		}
		if err := fileScanner.ScanFile(ctx, path, sink); err != nil {
			log.Printf("source %s: scan %s: %v", src.Descriptor().ID, path, err)
		}
	})

	if *syncMode == "fsnotify" {
		for _, src := range registry.All() {
			for _, dir := range src.WatchDirs() {
				if _, err := os.Stat(dir); err == nil {
					_ = w.WatchDirRecursive(dir)
				}
			}
		}
	}

	// Background scan loop. Sources own their own incremental cadence; the
	// JSONL runner does a full directory walk once a minute and cheap
	// known-file scans in between.
	go func() {
		scanAll := func() {
			for _, src := range registry.All() {
				if err := src.Scan(ctx, sink); err != nil {
					log.Printf("source %s scan failed: %v", src.Descriptor().ID, err)
				}
			}
		}
		scanAll()
		switch *syncMode {
		case "once":
			return
		case "fsnotify":
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !w.IsPaused() {
						scanAll()
					}
				}
			}
		default:
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !w.IsPaused() {
						scanAll()
					}
				}
			}
		}
	}()

	// Background goroutine shared by web/gui modes: persist remote LAN events.
	// Local usage is already written by the source sink.
	startDBDrain := func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case u, ok := <-w.UsageChan:
					if !ok {
						return
					}
					isLocal := (u.DeviceID == "" || u.DeviceID == *deviceID)

					if !isLocal {
						cost, _ := calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)

						effectiveDevice := u.DeviceID
						if effectiveDevice == "" {
							effectiveDevice = *deviceID
						}

						if err := database.InsertUsage(u, cost, effectiveDevice); err != nil {
							log.Printf("Failed to insert usage for device %s: %v", effectiveDevice, err)
							continue
						}
					}
					var activeLAN *lan.LAN
					if lanController != nil {
						activeLAN = lanController.CurrentLAN()
					} else {
						activeLAN = lanInst
					}
					if isLocal && activeLAN != nil {
						activeLAN.AnnounceDirty()
					}
				}
			}
		}()
	}

	if runGui {
		lanController = newRuntimeLANController(ctx, *lanMode, *deviceID, *port, *token, database, w.BroadcastChan, w.UsageChan, startLANHTTPServer, startLANGoroutines)
		startDBDrain()

		app := desktop.NewApp(database, calc)
		// Linux status-notifier support varies by desktop environment. On
		// Linux the window quits on close (see gui_lifecycle.go), so a tray is
		// optional; avoid opening a DBus status-notifier connection when it
		// would not be usable.
		if goruntime.GOOS != "linux" {
			tray := desktop.NewTray(app)
			tray.Start()
			defer tray.Stop()
		}

		// Build system tray / application menu
		appMenu := menu.NewMenu()
		fileMenu := appMenu.AddSubmenu("File")
		fileMenu.AddText("Show Dashboard", keys.CmdOrCtrl("d"), func(cd *menu.CallbackData) {
			runtime.WindowShow(app.GetContext())
		})
		fileMenu.AddSeparator()
		fileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(cd *menu.CallbackData) {
			runtime.Quit(app.GetContext())
		})

		if goos := goruntime.GOOS; goos == "darwin" {
			appMenu.Append(menu.EditMenu())
		}

		fmt.Println("🖥️  Starting AI Flight Dashboard (GUI mode)...")

		// Wails expects index.html at the FS root; strip the "dist" prefix
		guiAssets, err := fs.Sub(web.StaticFiles, "dist")
		if err != nil {
			log.Fatalf("Failed to create asset FS: %v", err)
		}

		if lanInst != nil {
			if _, err := lanController.Join(); err != nil {
				log.Printf("Failed to start LAN services: %v", err)
			}
		}

		// Push any post-save discovery config changes (extra peers, Tailscale
		// toggle) into the live LAN instance.
		web.SetConfigSavedHook(func(_ *config.AppConfig) {
			if lanController != nil {
				applyDiscoveryConfig(lanController.CurrentLAN())
			}
		})

		// Reuse the existing HTTP handler inside Wails.
		apiHandler := web.NewHandlerWithDeps(database, calc, w, lanController, *token, root.DistBinFS, web.HandlerDeps{
			Sources: registry,
			Plugins: pluginController,
		})

		guiOptions := &options.App{
			Title:     "AI Flight Dashboard",
			Width:     1280,
			Height:    800,
			MinWidth:  900,
			MinHeight: 600,
			AssetServer: &assetserver.Options{
				Assets:  guiAssets,
				Handler: apiHandler,
			},
			OnStartup: app.Startup,
			Menu:      appMenu,
			Bind: []interface{}{
				app,
			},
			Mac: &mac.Options{
				TitleBar:             mac.TitleBarHiddenInset(),
				WebviewIsTransparent: true,
				WindowIsTranslucent:  false,
				About: &mac.AboutInfo{
					Title:   "AI Flight Dashboard",
					Message: "Real-time AI token cost monitoring",
				},
			},
		}
		configureGUIWindowLifecycle(guiOptions, app)

		err = wailsrun.Run(guiOptions)
		if err != nil {
			log.Fatalf("GUI error: %v", err)
		}
		return
	}

	if *webMode {
		lanController = newRuntimeLANController(ctx, *lanMode, *deviceID, *port, *token, database, w.BroadcastChan, w.UsageChan, nil, startLANGoroutines)
		startDBDrain()

		web.SetConfigSavedHook(func(_ *config.AppConfig) {
			if lanController != nil {
				applyDiscoveryConfig(lanController.CurrentLAN())
			}
		})

		// Web dashboard mode with graceful shutdown
		handler := web.NewHandlerWithDeps(database, calc, w, lanController, *token, root.DistBinFS, web.HandlerDeps{
			Sources: registry,
			Plugins: pluginController,
		})
		srv := &http.Server{Addr: "0.0.0.0:" + *port, Handler: handler}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			log.Fatalf("Web server error: %v", err)
		}
		if lanInst != nil {
			if _, err := lanController.Join(); err != nil {
				log.Printf("Failed to start LAN services: %v", err)
			}
		}

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			fmt.Println("\n🛬 Shutting down gracefully...")
			srv.Shutdown(context.Background())
		}()

		fmt.Printf("🌐 Web dashboard: http://localhost:%s\n", *port)
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			log.Fatalf("Web server error: %v", err)
		}
		return
	}

	// TUI mode — starts instantly, data populates in background
	homeDirTui, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDirTui, ".ai-flight-dashboard", "stats", "debug.log")
	os.MkdirAll(filepath.Dir(logPath), 0755)
	f, err := tea.LogToFile(logPath, "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer f.Close()

	// Budget alert: active in api mode with a daily limit
	var budgetAlert *alert.BudgetAlert
	if billingMode == model.BillingAPI && *budgetDaily > 0 {
		budgetAlert = alert.NewBudgetAlert(database, *budgetDaily)
		fmt.Printf("💰 Budget mode: $%.2f/day limit\n", *budgetDaily)
	}
	if lanInst != nil {
		if !startLocalLANServices(ctx, lanInst, database, *token, *port, w.BroadcastChan, w.UsageChan, startLANHTTPServer, startLANGoroutines) {
			lanInst = nil
		}
	}

	p := tea.NewProgram(tui.NewModel(calc, w, database, budgetAlert))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
