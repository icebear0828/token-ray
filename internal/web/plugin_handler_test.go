package web_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-flight-dashboard/internal/plugin"
	"ai-flight-dashboard/internal/source"
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
