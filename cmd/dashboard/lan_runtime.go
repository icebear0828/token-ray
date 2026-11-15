package main

import (
	"token-ray/internal/config"
	"token-ray/internal/dashboard"
	"token-ray/internal/db"
	"token-ray/internal/lan"
	"token-ray/internal/model"
	"token-ray/internal/web"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

func startLANGoroutines(ctx context.Context, lanInst *lan.LAN, database *db.DB, token string, broadcastChan <-chan model.TokenUsage, usageChan chan<- model.TokenUsage) {
	fmt.Printf("📡 LAN discovery enabled. Multicast: %s\n", lan.MulticastAddr)
	lanInst.SetSummaryProvider(func() model.TokenSummary {
		summary, err := dashboard.BuildTokenSummary(database, lanInst.DeviceID)
		if err != nil {
			return model.TokenSummary{}
		}
		return summary
	})
	go lanInst.StartListenerContext(ctx, usageChan)
	go lanInst.StartPingerContext(ctx)
	go lanInst.StartHTTPDiscovery(ctx)
	go lanInst.StartBroadcasterContext(ctx, broadcastChan)
	go lanInst.StartAutoSyncContext(ctx, database, token)
	if token == "" {
		fmt.Println("🌐 Zero-config LAN sync enabled for private networks.")
		return
	}
}

type lanHTTPServerHandle struct {
	done <-chan struct{}
}

func startLANHTTPServer(ctx context.Context, port string, handler http.Handler) (*lanHTTPServerHandle, bool) {
	addr := "0.0.0.0:" + port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("LAN API server unavailable on port %s: %v", port, err)
		return nil, false
	}
	srv := &http.Server{Handler: handler}
	done := make(chan struct{})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	go func() {
		defer close(done)
		fmt.Printf("🌐 LAN API server: http://0.0.0.0:%s\n", port)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("LAN API server unavailable on port %s: %v", port, err)
		}
	}()
	return &lanHTTPServerHandle{done: done}, true
}

type lanHTTPServerStarter func(context.Context, string, http.Handler) (*lanHTTPServerHandle, bool)
type lanRuntimeStarter func(context.Context, *lan.LAN, *db.DB, string, <-chan model.TokenUsage, chan<- model.TokenUsage)

const lanHTTPShutdownWait = 4 * time.Second

type runtimeLANController struct {
	mu            sync.RWMutex
	parentCtx     context.Context
	lanMode       bool
	deviceID      string
	port          string
	token         string
	database      *db.DB
	broadcastChan <-chan model.TokenUsage
	usageChan     chan<- model.TokenUsage
	startHTTP     lanHTTPServerStarter
	startRuntime  lanRuntimeStarter
	cancel        context.CancelFunc
	httpDone      <-chan struct{}
	lanInst       *lan.LAN
}

func newRuntimeLANController(
	parentCtx context.Context,
	lanMode bool,
	deviceID string,
	port string,
	token string,
	database *db.DB,
	broadcastChan <-chan model.TokenUsage,
	usageChan chan<- model.TokenUsage,
	startHTTP lanHTTPServerStarter,
	startRuntime lanRuntimeStarter,
) *runtimeLANController {
	return &runtimeLANController{
		parentCtx:     parentCtx,
		lanMode:       lanMode,
		deviceID:      deviceID,
		port:          port,
		token:         token,
		database:      database,
		broadcastChan: broadcastChan,
		usageChan:     usageChan,
		startHTTP:     startHTTP,
		startRuntime:  startRuntime,
	}
}
func (c *runtimeLANController) CurrentLAN() *lan.LAN {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lanInst
}
func (c *runtimeLANController) Status() model.LANStatusResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return model.LANStatusResponse{Enabled: c.lanInst != nil}
}
func (c *runtimeLANController) Join() (model.LANStatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lanInst != nil {
		c.lanInst.Ping()
		if err := c.saveLANSetting(true); err != nil {
			return model.LANStatusResponse{Enabled: true}, err
		}
		return model.LANStatusResponse{Enabled: true}, nil
	}
	enabled := true
	lanInst := newLANInstance(c.lanMode, &enabled, c.token, c.deviceID, c.port)
	if lanInst == nil {
		return model.LANStatusResponse{Enabled: false}, fmt.Errorf("LAN is disabled by launch configuration")
	}
	if err := c.saveLANSetting(true); err != nil {
		return model.LANStatusResponse{Enabled: false}, err
	}

	runtimeCtx, cancel := context.WithCancel(c.parentCtx)
	var httpHandle *lanHTTPServerHandle
	if c.startHTTP != nil {
		var ok bool
		httpHandle, ok = c.startHTTP(runtimeCtx, c.port, web.NewLANHandler(c.database, c.token, lanInst))
		if !ok {
			cancel()
			if err := c.saveLANSetting(false); err != nil {
				return model.LANStatusResponse{Enabled: false}, err
			}
			return model.LANStatusResponse{Enabled: false}, fmt.Errorf("failed to start LAN HTTP server")
		}
	}
	c.startRuntime(runtimeCtx, lanInst, c.database, c.token, c.broadcastChan, c.usageChan)

	c.lanInst = lanInst
	c.cancel = cancel
	if httpHandle != nil {
		c.httpDone = httpHandle.done
	} else {
		c.httpDone = nil
	}
	return model.LANStatusResponse{Enabled: true}, nil
}
func (c *runtimeLANController) Leave() (model.LANStatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lanInst == nil {
		if err := c.saveLANSetting(false); err != nil {
			return model.LANStatusResponse{Enabled: false}, err
		}
		return model.LANStatusResponse{Enabled: false}, nil
	}
	if err := c.saveLANSetting(false); err != nil {
		return model.LANStatusResponse{Enabled: true}, err
	}

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if err := waitForLANHTTPShutdown(c.httpDone); err != nil {
		return model.LANStatusResponse{Enabled: true}, err
	}
	c.httpDone = nil
	c.lanInst = nil
	return model.LANStatusResponse{Enabled: false}, nil
}
func waitForLANHTTPShutdown(done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(lanHTTPShutdownWait):
		return fmt.Errorf("timed out waiting for LAN HTTP server shutdown")
	}
}
func (c *runtimeLANController) saveLANSetting(enabled bool) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load LAN config: %w", err)
	}
	cfg.EnableLAN = &enabled
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save LAN config: %w", err)
	}
	return nil
}
func startLocalLANServices(
	ctx context.Context,
	lanInst *lan.LAN,
	database *db.DB,
	token string,
	port string,
	broadcastChan <-chan model.TokenUsage,
	usageChan chan<- model.TokenUsage,
	startHTTP lanHTTPServerStarter,
	startRuntime lanRuntimeStarter,
) bool {
	if lanInst == nil {
		return false
	}
	if _, ok := startHTTP(ctx, port, web.NewLANHandler(database, token, lanInst)); !ok {
		return false
	}
	startRuntime(ctx, lanInst, database, token, broadcastChan, usageChan)
	return true
}
func newLANInstance(lanMode bool, enableLAN *bool, token string, deviceID string, port string) *lan.LAN {
	if !lanMode {
		return nil
	}
	if enableLAN != nil && !*enableLAN {
		return nil
	}
	portInt, _ := strconv.Atoi(port)
	if portInt == 0 {
		portInt = lan.DefaultHTTPPort
	}
	lanInst := lan.New(deviceID, portInt)
	lanInst.SetHTTPDiscoveryPorts(portInt)
	applyDiscoveryConfig(lanInst)
	return lanInst
}

// applyDiscoveryConfig reads the current config and pushes the extra-host
// settings into the LAN instance. Safe to call any time the config changes.
// Tailscale auto-discovery defaults to ON when the field is absent so that
// existing users on Tailscale see peers without changing their config.
func applyDiscoveryConfig(lanInst *lan.LAN) {
	if lanInst == nil {
		return
	}
	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil {
		lanInst.SetExtraPeerHosts(nil)
		lanInst.SetTailscaleDiscovery(true)
		return
	}
	lanInst.SetExtraPeerHosts(cfg.ExtraPeerHosts)
	tailscale := true
	if cfg.TailscaleDiscovery != nil {
		tailscale = *cfg.TailscaleDiscovery
	}
	lanInst.SetTailscaleDiscovery(tailscale)
}
