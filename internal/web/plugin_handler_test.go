package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/plugin"
	"ai-flight-dashboard/internal/source"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
)

type fakeSources struct{}

func (fakeSources) Descriptors() []source.Descriptor {
	return []source.Descriptor{{ID: "demo", Source: "Demo", Label: "DEMO"}}
}

type fakePlugins struct {
	installed []plugin.Installed
	removed   string
	enabled   map[string]bool
}

func (f *fakePlugins) List() ([]plugin.Installed, error) { return f.installed, nil }
func (f *fakePlugins) InstallZip(io.Reader) (plugin.Manifest, error) {
	return plugin.Manifest{ID: "demo"}, nil
}
func (f *fakePlugins) InstallURL(context.Context, string, string) (plugin.Manifest, error) {
	return plugin.Manifest{ID: "demo"}, nil
}
func (f *fakePlugins) Remove(id string) error {
	f.removed = id
	return nil
}
func (f *fakePlugins) SetEnabled(id string, enabled bool) error {
	if f.enabled == nil {
		f.enabled = map[string]bool{}
	}
	f.enabled[id] = enabled
	return nil
}
func (f *fakePlugins) FetchIndex(context.Context) ([]byte, error) {
	return []byte(`{"plugins":[]}`), nil
}

func newTestHandler(sources web.SourceProvider, plugins web.PluginController) http.Handler {
	return web.NewHandlerWithDeps(nil, nil, nil, nil, "", emptyFS, web.HandlerDeps{
		Sources: sources,
		Plugins: plugins,
	})
}

func TestSourcesEndpoint(t *testing.T) {
	handler := newTestHandler(fakeSources{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var descriptors []source.Descriptor
	if err := json.NewDecoder(rec.Body).Decode(&descriptors); err != nil {
		t.Fatal(err)
	}
	if len(descriptors) != 1 || descriptors[0].ID != "demo" {
		t.Fatalf("unexpected descriptors: %+v", descriptors)
	}
}

func TestPluginsListEndpoint(t *testing.T) {
	plugins := &fakePlugins{installed: []plugin.Installed{{Manifest: plugin.Manifest{ID: "demo", Name: "Demo"}, Enabled: true}}}
	handler := newTestHandler(nil, plugins)
	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var installed []plugin.Installed
	if err := json.NewDecoder(rec.Body).Decode(&installed); err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0].Manifest.ID != "demo" {
		t.Fatalf("unexpected installed plugins: %+v", installed)
	}
}

func TestPluginEnableDisableAndRemove(t *testing.T) {
	plugins := &fakePlugins{}
	handler := newTestHandler(nil, plugins)

	for _, action := range []string{"enable", "disable"} {
		req := httptest.NewRequest(http.MethodPost, "/api/plugins/demo/"+action, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: unexpected status: %d", action, rec.Code)
		}
	}
	if plugins.enabled["demo"] {
		t.Fatal("expected final disable state to be false")
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/demo", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: unexpected status: %d", rec.Code)
	}
	if plugins.removed != "demo" {
		t.Fatalf("expected demo to be removed, got %q", plugins.removed)
	}
}

func TestPluginIndexEndpoint(t *testing.T) {
	handler := newTestHandler(nil, &fakePlugins{})
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/index", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestPluginMutationsInvalidateStatsCache(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", OutputTokens: 1000, UUID: "seed-1"},
		1.00, now.Add(-5*time.Minute), "/a.jsonl", "dev-a",
	); err != nil {
		t.Fatal(err)
	}

	plugins := &fakePlugins{}
	statsCache := dashboard.NewStatsCache(time.Minute)
	handler := web.NewHandlerWithDeps(database, calc, nil, nil, "", emptyFS, web.HandlerDeps{
		Sources:    fakeSources{},
		Plugins:    plugins,
		StatsCache: statsCache,
	})

	fetchSummary := func() (string, model.StatsResponse) {
		req := httptest.NewRequest(http.MethodGet, "/api/stats?detail=summary&period=30d", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/stats status=%d body=%s", rec.Code, rec.Body.String())
		}
		var parsed model.StatsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("unmarshal stats: %v", err)
		}
		return rec.Header().Get("ETag"), parsed
	}

	makeZipUploadReq := func() *http.Request {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", "plugin.zip")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte("PK\x05\x06"))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/plugins/install", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return req
	}

	mutations := []struct {
		name string
		req  func() *http.Request
	}{
		{
			name: "disable plugin",
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/plugins/demo/disable", nil)
			},
		},
		{
			name: "enable plugin",
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/plugins/demo/enable", nil)
			},
		},
		{
			name: "remove plugin",
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/api/plugins/demo", nil)
			},
		},
		{
			name: "install plugin via URL",
			req: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/api/plugins/install", strings.NewReader(`{"url":"https://example.com/demo.zip"}`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
		},
		{
			name: "install plugin via zip upload",
			req:  makeZipUploadReq,
		},
	}

	for i, tc := range mutations {
		etagBefore, _ := fetchSummary()

		if err := database.InsertUsageWithTime(
			model.TokenUsage{Source: "Codex", Model: "gpt-5.5", OutputTokens: 500, UUID: "mut-" + tc.name},
			2.00, now.Add(-time.Duration(4-i)*time.Minute), "/b.jsonl", "dev-"+tc.name,
		); err != nil {
			t.Fatal(err)
		}

		// Sanity check: cache is still warm before mutation.
		etagWarm, _ := fetchSummary()
		if etagWarm != etagBefore {
			t.Fatalf("%s: expected stats cache warm before mutation, got %q vs %q", tc.name, etagWarm, etagBefore)
		}

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, tc.req())
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}

		etagAfter, statsAfter := fetchSummary()
		if etagAfter == etagBefore {
			t.Fatalf("%s: expected statsCache invalidated and ETag changed, still got %q", tc.name, etagAfter)
		}
		if len(statsAfter.Devices) != i+2 {
			t.Fatalf("%s: expected %d devices after invalidation, got %d", tc.name, i+2, len(statsAfter.Devices))
		}
	}
}
