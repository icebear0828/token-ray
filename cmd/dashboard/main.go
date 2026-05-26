package main

import (
	root "ai-flight-dashboard"
	"ai-flight-dashboard/internal/agyscanner"
	"ai-flight-dashboard/internal/alert"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/desktop"
	"ai-flight-dashboard/internal/forwarder"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/scanner"
	"ai-flight-dashboard/internal/tui"
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

	processLock := acquireProcessLock(appDataDir)
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
	// Collect scan directories
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

	// Fast path: register cached known directories (~1ms vs 134ms recursive)
	knownDirs, _ := database.ListKnownDirs()
	if *syncMode == "fsnotify" && len(knownDirs) > 0 {
		w.WatchKnownDirs(knownDirs)
		// Also watch roots recursively so new Gemini/Claude session subdirs are not missed.
		for _, dir := range scanDirs {
			w.WatchDirRecursive(dir)
		}
	}

	// Background: full scan + discover new directories
	go func() {
		s := scanner.New(database, calc, *deviceID)
		codexScanner := codexusage.New(database, calc, *deviceID)
		agyScanner := agyscanner.New(database, calc, *deviceID)
		s.ScanAll(scanDirs, w.UsageChan) // incremental; auto-caches new dirs to known_dirs
		codexScanner.Scan(w.UsageChan)
		agyScanner.Scan(w.UsageChan)

		if *syncMode == "fsnotify" {
			if len(knownDirs) == 0 {
				// First run: no cache yet, do full recursive watch
				for _, dir := range scanDirs {
					w.WatchDirRecursive(dir)
				}
			} else {
				// Subsequent runs: pick up any newly discovered dirs
				newDirs, _ := database.ListKnownDirs()
				w.WatchKnownDirs(newDirs)
			}
			codexTicker := time.NewTicker(30 * time.Second)
			defer codexTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-codexTicker.C:
					if !w.IsPaused() {
						codexScanner.Scan(w.UsageChan)
						agyScanner.Scan(w.UsageChan)
					}
				}
			}
		} else if *syncMode == "poll" {
			fastTicker := time.NewTicker(2 * time.Second)
			slowTicker := time.NewTicker(60 * time.Second)
			defer fastTicker.Stop()
			defer slowTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-fastTicker.C:
					if !w.IsPaused() {
						s.ScanKnownFiles(w.UsageChan)
						codexScanner.Scan(w.UsageChan)
						agyScanner.Scan(w.UsageChan)
					}
				case <-slowTicker.C:
					if !w.IsPaused() {
						s.ScanAll(scanDirs, w.UsageChan)
					}
				}
			}
		}
		// "once" mode does nothing more
	}()

	// Background goroutine shared by web/gui modes: drain watcher events and persist to DB
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

					// fsnotify events are inserted here; poll scanners already inserted
					// their records before sending usage notifications.
					if !isLocal || *syncMode == "fsnotify" {
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
		apiHandler := web.NewHandlerWithLANController(database, calc, w, lanController, *token, root.DistBinFS)

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
		handler := web.NewHandlerWithLANController(database, calc, w, lanController, *token, root.DistBinFS)
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

	skipDBWrite := (*syncMode != "fsnotify")
	p := tea.NewProgram(tui.NewModel(calc, w, database, budgetAlert, skipDBWrite))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
