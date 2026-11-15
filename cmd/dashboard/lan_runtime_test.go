package main

import (
	"token-ray/internal/config"
	"token-ray/internal/db"
	"token-ray/internal/lan"
	"token-ray/internal/model"
	"token-ray/internal/testutil"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLANInstanceAdvertisesSyncPortWithoutTokenForZeroConfigLAN(t *testing.T) {
	enabled := true
	lanInst := newLANInstance(true, &enabled, "", "local-device", "19100")
	if lanInst == nil {
		t.Fatal("expected zero-config LAN instance without token")
	}
	if lanInst.DeviceID != "local-device" {
		t.Fatalf("unexpected device ID: %q", lanInst.DeviceID)
	}
	if lanInst.HTTPPort != 19100 {
		t.Fatalf("expected advertised zero-config sync port without token, got %d", lanInst.HTTPPort)
	}
}
func TestNewLANInstanceDefaultsMissingSettingToEnabled(t *testing.T) {
	lanInst := newLANInstance(true, nil, "", "local-device", "19100")
	if lanInst == nil {
		t.Fatal("expected LAN discovery to default on when setting is missing")
	}
	if lanInst.DeviceID != "local-device" {
		t.Fatalf("unexpected device ID: %q", lanInst.DeviceID)
	}
}
func TestNewLANInstanceAdvertisesSyncPortWithToken(t *testing.T) {
	enabled := true
	lanInst := newLANInstance(true, &enabled, "secret-token", "local-device", "19100")
	if lanInst == nil {
		t.Fatal("expected LAN instance with token")
	}
	if lanInst.HTTPPort != 19100 {
		t.Fatalf("expected advertised sync port, got %d", lanInst.HTTPPort)
	}
}
func TestNewLANInstanceDisabledBySettings(t *testing.T) {
	disabled := false
	if lanInst := newLANInstance(true, &disabled, "", "local-device", "19100"); lanInst != nil {
		t.Fatalf("expected LAN instance to be disabled, got %+v", lanInst)
	}
}
func TestStartLocalLANServicesStartsHTTPWithoutSyncToken(t *testing.T) {
	lanInst := lan.New("local-device", 19100)
	broadcastChan := make(chan model.TokenUsage)
	usageChan := make(chan model.TokenUsage)
	var capturedHandler http.Handler
	httpStarts := 0
	runtimeStarts := 0

	ok := startLocalLANServices(
		context.Background(),
		lanInst,
		nil,
		"",
		"19100",
		broadcastChan,
		usageChan,
		func(_ context.Context, port string, handler http.Handler) (*lanHTTPServerHandle, bool) {
			httpStarts++
			if port != "19100" {
				t.Fatalf("expected LAN HTTP port 19100, got %q", port)
			}
			capturedHandler = handler
			return nil, true
		},
		func(_ context.Context, gotLAN *lan.LAN, _ *db.DB, token string, gotBroadcast <-chan model.TokenUsage, gotUsage chan<- model.TokenUsage) {
			runtimeStarts++
			if gotLAN != lanInst {
				t.Fatalf("unexpected LAN instance: %+v", gotLAN)
			}
			if token != "" {
				t.Fatalf("expected empty sync token, got %q", token)
			}
			if gotBroadcast != broadcastChan {
				t.Fatal("unexpected broadcast channel")
			}
			if gotUsage != usageChan {
				t.Fatal("unexpected usage channel")
			}
		},
	)
	if !ok {
		t.Fatal("expected LAN services to start")
	}
	if httpStarts != 1 || runtimeStarts != 1 {
		t.Fatalf("expected one HTTP start and one runtime start, got http=%d runtime=%d", httpStarts, runtimeStarts)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lan/self", nil)
	rec := httptest.NewRecorder()
	capturedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected LAN self endpoint to be available without token, got %d", rec.Code)
	}
	var self model.LANSelfResponse
	if err := json.NewDecoder(rec.Body).Decode(&self); err != nil {
		t.Fatal(err)
	}
	if self.DeviceID != "local-device" || self.HTTPPort != 19100 {
		t.Fatalf("unexpected LAN self response: %+v", self)
	}
}
func TestRuntimeLANControllerJoinLeaveRestartsLANRuntime(t *testing.T) {
	configDir := t.TempDir()
	config.SetDataDir(configDir)
	defer config.SetDataDir("")

	database := testutil.NewTestDB(t)
	defer database.Close()

	broadcastChan := make(chan model.TokenUsage)
	usageChan := make(chan model.TokenUsage)
	httpStarts := 0
	runtimeStarts := 0
	var runtimeContexts []context.Context

	controller := newRuntimeLANController(
		context.Background(),
		true,
		"local-device",
		"19100",
		"",
		database,
		broadcastChan,
		usageChan,
		func(ctx context.Context, port string, handler http.Handler) (*lanHTTPServerHandle, bool) {
			httpStarts++
			runtimeContexts = append(runtimeContexts, ctx)
			if port != "19100" {
				t.Fatalf("expected port 19100, got %q", port)
			}
			if handler == nil {
				t.Fatal("expected LAN HTTP handler")
			}
			return nil, true
		},
		func(ctx context.Context, gotLAN *lan.LAN, gotDB *db.DB, token string, gotBroadcast <-chan model.TokenUsage, gotUsage chan<- model.TokenUsage) {
			runtimeStarts++
			runtimeContexts = append(runtimeContexts, ctx)
			if gotLAN == nil || gotLAN.DeviceID != "local-device" {
				t.Fatalf("unexpected LAN instance: %+v", gotLAN)
			}
			if gotDB != database {
				t.Fatal("unexpected database")
			}
			if token != "" {
				t.Fatalf("expected empty token, got %q", token)
			}
			if gotBroadcast != broadcastChan {
				t.Fatal("unexpected broadcast channel")
			}
			if gotUsage != usageChan {
				t.Fatal("unexpected usage channel")
			}
		},
	)

	status, err := controller.Join()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || controller.CurrentLAN() == nil {
		t.Fatalf("expected LAN enabled after join, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	if httpStarts != 1 || runtimeStarts != 1 {
		t.Fatalf("expected one HTTP and runtime start, got http=%d runtime=%d", httpStarts, runtimeStarts)
	}

	status, err = controller.Leave()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || controller.CurrentLAN() != nil {
		t.Fatalf("expected LAN disabled after leave, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	for _, ctx := range runtimeContexts {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("expected runtime context to be canceled after leave")
		}
	}

	status, err = controller.Join()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || controller.CurrentLAN() == nil {
		t.Fatalf("expected LAN enabled after rejoin, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	if httpStarts != 2 || runtimeStarts != 2 {
		t.Fatalf("expected LAN runtime to restart, got http=%d runtime=%d", httpStarts, runtimeStarts)
	}
}
func TestRuntimeLANControllerLeaveWaitsForHTTPShutdownBeforeRejoin(t *testing.T) {
	configDir := t.TempDir()
	config.SetDataDir(configDir)
	defer config.SetDataDir("")

	database := testutil.NewTestDB(t)
	defer database.Close()

	httpRelease := make(chan struct{})
	httpCancelSeen := make(chan struct{}, 1)
	httpStarts := 0
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller := newRuntimeLANController(
		parentCtx,
		true,
		"local-device",
		"19100",
		"",
		database,
		make(chan model.TokenUsage),
		make(chan model.TokenUsage),
		func(ctx context.Context, port string, handler http.Handler) (*lanHTTPServerHandle, bool) {
			httpStarts++
			done := make(chan struct{})
			go func() {
				<-ctx.Done()
				httpCancelSeen <- struct{}{}
				<-httpRelease
				close(done)
			}()
			return &lanHTTPServerHandle{done: done}, true
		},
		func(context.Context, *lan.LAN, *db.DB, string, <-chan model.TokenUsage, chan<- model.TokenUsage) {},
	)

	if _, err := controller.Join(); err != nil {
		t.Fatal(err)
	}

	leaveDone := make(chan error, 1)
	go func() {
		_, err := controller.Leave()
		leaveDone <- err
	}()

	select {
	case <-httpCancelSeen:
	case <-time.After(time.Second):
		t.Fatal("expected leave to cancel the HTTP server context")
	}
	select {
	case err := <-leaveDone:
		t.Fatalf("leave returned before HTTP shutdown completed: %v", err)
	default:
	}

	close(httpRelease)
	if err := <-leaveDone; err != nil {
		t.Fatal(err)
	}

	if _, err := controller.Join(); err != nil {
		t.Fatal(err)
	}
	if httpStarts != 2 {
		t.Fatalf("expected HTTP server to start twice, got %d", httpStarts)
	}
}
func TestRuntimeLANControllerDoesNotStartWhenLANConfigCannotPersist(t *testing.T) {
	configDir := t.TempDir()
	blocker := filepath.Join(configDir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocked"), 0644); err != nil {
		t.Fatal(err)
	}
	config.SetDataDir(blocker)
	defer config.SetDataDir("")

	database := testutil.NewTestDB(t)
	defer database.Close()

	runtimeStarts := 0
	controller := newRuntimeLANController(
		context.Background(),
		true,
		"local-device",
		"19100",
		"",
		database,
		make(chan model.TokenUsage),
		make(chan model.TokenUsage),
		nil,
		func(context.Context, *lan.LAN, *db.DB, string, <-chan model.TokenUsage, chan<- model.TokenUsage) {
			runtimeStarts++
		},
	)

	status, err := controller.Join()
	if err == nil {
		t.Fatal("expected join to fail when LAN config cannot be persisted")
	}
	if status.Enabled || controller.CurrentLAN() != nil {
		t.Fatalf("expected LAN to remain disabled after config failure, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	if runtimeStarts != 0 {
		t.Fatalf("expected runtime not to start when config cannot persist, got %d starts", runtimeStarts)
	}
}
func TestRuntimeLANControllerDoesNotLeaveWhenLANConfigCannotPersist(t *testing.T) {
	validConfigDir := t.TempDir()
	config.SetDataDir(validConfigDir)
	defer config.SetDataDir("")

	database := testutil.NewTestDB(t)
	defer database.Close()

	controller := newRuntimeLANController(
		context.Background(),
		true,
		"local-device",
		"19100",
		"",
		database,
		make(chan model.TokenUsage),
		make(chan model.TokenUsage),
		nil,
		func(context.Context, *lan.LAN, *db.DB, string, <-chan model.TokenUsage, chan<- model.TokenUsage) {},
	)
	if _, err := controller.Join(); err != nil {
		t.Fatal(err)
	}

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocked"), 0644); err != nil {
		t.Fatal(err)
	}
	config.SetDataDir(blocker)

	status, err := controller.Leave()
	if err == nil {
		t.Fatal("expected leave to fail when LAN config cannot be persisted")
	}
	if !status.Enabled || controller.CurrentLAN() == nil {
		t.Fatalf("expected LAN to remain enabled after config failure, status=%+v current=%+v", status, controller.CurrentLAN())
	}
}
