package forwarder_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"token-ray/internal/forwarder"
	"token-ray/internal/model"
)

func TestForwarder_Success(t *testing.T) {
	var receivedPayload model.TrackPayload
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	fw := forwarder.New(srv.URL, "test-token", "test-device")

	ch := make(chan model.TokenUsage, 1)
	ch <- model.TokenUsage{
		Source:       "Test",
		Model:        "test-model",
		InputTokens:  10,
		OutputTokens: 20,
	}
	close(ch)

	// Run forwarder synchronously
	fw.Start(ch)

	if authHeader != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %s", authHeader)
	}
	if receivedPayload.DeviceID != "test-device" {
		t.Errorf("expected test-device, got %s", receivedPayload.DeviceID)
	}
	if receivedPayload.Usage.Model != "test-model" {
		t.Errorf("expected test-model, got %s", receivedPayload.Usage.Model)
	}
}

func TestForwarder_Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	fw := forwarder.New(srv.URL, "", "test-device")

	ch := make(chan model.TokenUsage, 1)
	ch <- model.TokenUsage{Model: "test-model"}
	close(ch)

	start := time.Now()
	fw.Start(ch)
	dur := time.Since(start)

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if dur < 2*time.Second {
		t.Errorf("expected retry delay, took %v", dur)
	}
}
