package updater_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"ai-flight-dashboard/internal/updater"
)

func TestCheckForUpdates_SemanticComparison(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		remoteTag      string
		wantUpdate     bool
	}{
		{
			name:           "newer remote patch",
			currentVersion: "v1.8.1",
			remoteTag:      "v1.8.2",
			wantUpdate:     true,
		},
		{
			name:           "newer remote minor",
			currentVersion: "v1.8.1",
			remoteTag:      "v1.9.0",
			wantUpdate:     true,
		},
		{
			name:           "same version with and without v prefix",
			currentVersion: "v1.8.1",
			remoteTag:      "1.8.1",
			wantUpdate:     false,
		},
		{
			name:           "older remote version",
			currentVersion: "v1.8.2",
			remoteTag:      "v1.8.1",
			wantUpdate:     false,
		},
		{
			name:           "development or zero local version",
			currentVersion: "dev",
			remoteTag:      "v1.8.1",
			wantUpdate:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updater.IsNewerVersion(tt.currentVersion, tt.remoteTag)
			if got != tt.wantUpdate {
				t.Errorf("IsNewerVersion(%q, %q) = %v; want %v", tt.currentVersion, tt.remoteTag, got, tt.wantUpdate)
			}
		})
	}
}

func TestCheckForUpdates_HTTPClient(t *testing.T) {
	mockRelease := updater.Release{
		TagName: "v1.9.0",
		Name:    "v1.9.0 Release",
		Body:    "## Highlights\n- Automated updates",
		Assets: []updater.Asset{
			{Name: "ai-flight-dashboard-macos-m-series.tar.gz", URL: "https://example.com/mac"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockRelease)
	}))
	defer srv.Close()

	client := updater.NewClient(
		updater.WithBaseURL(srv.URL),
		updater.WithRepo("test/repo"),
	)

	// Test update found
	rel, err := client.CheckForUpdates("v1.8.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected release, got nil")
	}
	if rel.TagName != "v1.9.0" {
		t.Errorf("expected tag v1.9.0, got %s", rel.TagName)
	}

	// Test already up-to-date
	relSame, err := client.CheckForUpdates("v1.9.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if relSame != nil {
		t.Errorf("expected nil when up to date, got %+v", relSame)
	}
}

func TestMatchAsset(t *testing.T) {
	assets := []updater.Asset{
		{Name: "ai-flight-dashboard-linux.tar.gz", URL: "https://example.com/linux-tar"},
		{Name: "ai-flight-dashboard-macos-m-series.tar.gz", URL: "https://example.com/mac-arm64"},
		{Name: "ai-flight-dashboard-macos-intel.tar.gz", URL: "https://example.com/mac-amd64"},
		{Name: "ai-flight-dashboard-windows.zip", URL: "https://example.com/win-zip"},
		{Name: "ai-flight-dashboard-linux-amd64", URL: "https://example.com/linux-bin"},
	}

	// For linux amd64, prefer raw binary if present
	linuxMatch := updater.MatchAsset(assets, "linux", "amd64")
	if linuxMatch == nil || linuxMatch.Name != "ai-flight-dashboard-linux-amd64" {
		t.Fatalf("expected raw binary match for linux amd64, got %+v", linuxMatch)
	}

	// For darwin arm64, match macos-m-series.tar.gz
	macArmMatch := updater.MatchAsset(assets, "darwin", "arm64")
	if macArmMatch == nil || macArmMatch.Name != "ai-flight-dashboard-macos-m-series.tar.gz" {
		t.Fatalf("expected macos-m-series match for darwin arm64, got %+v", macArmMatch)
	}

	// For darwin amd64, match macos-intel.tar.gz
	macAmdMatch := updater.MatchAsset(assets, "darwin", "amd64")
	if macAmdMatch == nil || macAmdMatch.Name != "ai-flight-dashboard-macos-intel.tar.gz" {
		t.Fatalf("expected macos-intel match for darwin amd64, got %+v", macAmdMatch)
	}

	// For windows amd64, match windows.zip
	winMatch := updater.MatchAsset(assets, "windows", "amd64")
	if winMatch == nil || winMatch.Name != "ai-flight-dashboard-windows.zip" {
		t.Fatalf("expected windows.zip match for windows amd64, got %+v", winMatch)
	}

	// Unknown architecture returns nil
	unknownMatch := updater.MatchAsset(assets, "freebsd", "riscv64")
	if unknownMatch != nil {
		t.Fatalf("expected nil for unsupported platform, got %+v", unknownMatch)
	}
}

func TestExtractBinary_TarGz(t *testing.T) {
	expectedContent := []byte("#!/bin/sh\necho 'hello from test binary'\n")

	// Create in-memory tar.gz containing an app bundle structure
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add dummy README
	readme := []byte("readme content")
	_ = tw.WriteHeader(&tar.Header{
		Name:     "README.txt",
		Mode:     0644,
		Size:     int64(len(readme)),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(readme)

	// Add target binary in macOS App bundle
	_ = tw.WriteHeader(&tar.Header{
		Name:     "AI Flight Dashboard.app/Contents/MacOS/ai-flight-dashboard",
		Mode:     0755,
		Size:     int64(len(expectedContent)),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(expectedContent)

	_ = tw.Close()
	_ = gw.Close()

	extracted, err := updater.ExtractBinaryFromArchive(&buf, "ai-flight-dashboard-macos-m-series.tar.gz")
	if err != nil {
		t.Fatalf("failed to extract from tar.gz: %v", err)
	}

	gotBytes, err := io.ReadAll(extracted)
	if err != nil {
		t.Fatalf("failed to read extracted bytes: %v", err)
	}

	if !bytes.Equal(gotBytes, expectedContent) {
		t.Errorf("extracted content mismatch; got %q, want %q", string(gotBytes), string(expectedContent))
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	expectedContent := []byte("MZ Windows Binary Executable Content")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Add license
	licW, _ := zw.Create("LICENSE.txt")
	_, _ = licW.Write([]byte("MIT License"))

	// Add executable
	exeW, _ := zw.Create("ai-flight-dashboard.exe")
	_, _ = exeW.Write(expectedContent)

	_ = zw.Close()

	extracted, err := updater.ExtractBinaryFromArchive(&buf, "ai-flight-dashboard-windows.zip")
	if err != nil {
		t.Fatalf("failed to extract from zip: %v", err)
	}

	gotBytes, err := io.ReadAll(extracted)
	if err != nil {
		t.Fatalf("failed to read extracted bytes: %v", err)
	}

	if !bytes.Equal(gotBytes, expectedContent) {
		t.Errorf("extracted content mismatch; got %q, want %q", string(gotBytes), string(expectedContent))
	}
}

func TestExtractBinary_RawBinary(t *testing.T) {
	rawContent := []byte("raw executable bytes")
	buf := bytes.NewReader(rawContent)

	extracted, err := updater.ExtractBinaryFromArchive(buf, "ai-flight-dashboard-darwin-arm64")
	if err != nil {
		t.Fatalf("failed for raw binary: %v", err)
	}

	gotBytes, err := io.ReadAll(extracted)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	if !bytes.Equal(gotBytes, rawContent) {
		t.Errorf("content mismatch")
	}
}

func TestCurrentRuntimeAssetMatching(t *testing.T) {
	assets := []updater.Asset{
		{Name: "ai-flight-dashboard-linux.tar.gz", URL: "https://example.com/linux"},
		{Name: "ai-flight-dashboard-macos-m-series.tar.gz", URL: "https://example.com/mac-m"},
		{Name: "ai-flight-dashboard-macos-intel.tar.gz", URL: "https://example.com/mac-intel"},
		{Name: "ai-flight-dashboard-windows.zip", URL: "https://example.com/win"},
	}

	matched := updater.MatchAsset(assets, runtime.GOOS, runtime.GOARCH)
	if matched == nil {
		t.Fatalf("expected asset match for current platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	t.Logf("Matched asset for %s/%s: %s", runtime.GOOS, runtime.GOARCH, matched.Name)
}
