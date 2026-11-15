package web_test

import (
	"token-ray/internal/model"
	"token-ray/internal/testutil"
	"token-ray/internal/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDevicesAPIListsAliasesAndSupersedesDevice(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50, UUID: "old-device-row"},
		1.00, now, "/old.jsonl", "probe-local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "m2", InputTokens: 200, OutputTokens: 75, UUID: "real-device-row"},
		2.00, now, "/real.jsonl", "nas.local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDeviceAlias("nas.local", "NAS"); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.RemoteAddr = "127.0.0.1:42310"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected devices list, got %d", rec.Code)
	}
	var devices []model.DeviceSummary
	if err := json.NewDecoder(rec.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices, got %+v", devices)
	}
	if devices[0].ID != "nas.local" || devices[0].DisplayName != "NAS" {
		t.Fatalf("expected aliased nas.local first by cost, got %+v", devices)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/devices?device_id=probe-local", nil)
	deleteReq.RemoteAddr = "127.0.0.1:42310"
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected supersede success, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	var result model.DeviceSupersedeResponse
	if err := json.NewDecoder(deleteRec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.DeviceID != "probe-local" || result.SupersededCount != 1 {
		t.Fatalf("unexpected supersede response: %+v", result)
	}

	remaining, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0] != "nas.local" {
		t.Fatalf("expected probe-local hidden, got %+v", remaining)
	}
}

func TestDeviceAliasAPIDeletesAlias(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	if err := database.SetDeviceAlias("nas.local", "NAS"); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodDelete, "/api/device-alias?device_id=nas.local", nil)
	req.RemoteAddr = "127.0.0.1:42310"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected alias delete success, got %d", rec.Code)
	}

	aliases, err := database.GetDeviceAliases()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := aliases["nas.local"]; ok {
		t.Fatalf("expected alias to be deleted, got %+v", aliases)
	}
}
