package web_test

import (
	"token-ray/internal/testutil"
	"token-ray/internal/web"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPITrack(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	payload := `{"device_id":"remote-test","usage":{"source":"Claude Code","model":"claude-opus-4-7","input_tokens":100,"output_tokens":50}}`

	// Test unauthorized
	req, _ := http.NewRequest("POST", srv.URL+"/api/track", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test authorized
	req, _ = http.NewRequest("POST", srv.URL+"/api/track", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify it was inserted
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("remote-test", "")
	if cost == 0 {
		t.Fatal("expected cost to be calculated and inserted")
	}
}
