package plugin

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const demoManifestJSON = `{
  "schema_version": 1,
  "id": "demo",
  "name": "Demo CLI",
  "source": "Demo",
  "version": "1.0.0",
  "description": "A demo source",
  "watch_dirs": ["~/.demo/logs"],
  "fields": {
    "model": "model",
    "input_tokens": "usage.input",
    "output_tokens": "usage.output"
  }
}`

func TestManagerInstallListLoadRemove(t *testing.T) {
	mgr := NewManager(t.TempDir(), "1.0.0", nil)
	archive := buildZip(t, map[string]string{
		"demo/manifest.json": demoManifestJSON,
		"demo/README.md":     "hello",
	})

	manifest, err := mgr.InstallZip(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if manifest.ID != "demo" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}

	list, err := mgr.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Manifest.ID != "demo" || !list[0].Enabled {
		t.Fatalf("unexpected list: %+v", list)
	}

	sources := mgr.LoadSources()
	if len(sources) != 1 {
		t.Fatalf("expected one loaded source, got %d", len(sources))
	}
	if sources[0].Descriptor().Source != "Demo" {
		t.Fatalf("unexpected source: %+v", sources[0].Descriptor())
	}

	if err := mgr.Remove("demo"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	list, err = mgr.List()
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no plugins after remove, got %+v", list)
	}
}

func TestManagerRejectsPathTraversal(t *testing.T) {
	mgr := NewManager(t.TempDir(), "1.0.0", nil)
	archive := buildZip(t, map[string]string{
		"manifest.json": demoManifestJSON,
		"../evil.txt":   "bad",
	})
	if _, err := mgr.InstallZip(bytes.NewReader(archive)); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestManagerRejectsMissingManifest(t *testing.T) {
	mgr := NewManager(t.TempDir(), "1.0.0", nil)
	archive := buildZip(t, map[string]string{"README.md": "no manifest"})
	if _, err := mgr.InstallZip(bytes.NewReader(archive)); err == nil {
		t.Fatal("expected missing manifest to be rejected")
	}
}

func TestManagerRejectsIncompatibleVersion(t *testing.T) {
	mgr := NewManager(t.TempDir(), "1.0.0", nil)
	manifest := strings.Replace(demoManifestJSON, `"version": "1.0.0",`, `"version": "1.0.0", "min_app_version": "9.0.0",`, 1)
	archive := buildZip(t, map[string]string{"manifest.json": manifest})
	if _, err := mgr.InstallZip(bytes.NewReader(archive)); err == nil {
		t.Fatal("expected incompatible min_app_version to be rejected")
	}
}

func TestManagerDisabledPluginIsNotLoaded(t *testing.T) {
	mgr := NewManager(t.TempDir(), "1.0.0", func(id string) bool { return id != "demo" })
	archive := buildZip(t, map[string]string{"manifest.json": demoManifestJSON})
	if _, err := mgr.InstallZip(bytes.NewReader(archive)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if sources := mgr.LoadSources(); len(sources) != 0 {
		t.Fatalf("expected disabled plugin to be skipped, got %d sources", len(sources))
	}
	list, err := mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("expected plugin listed as disabled: %+v", list)
	}
}
