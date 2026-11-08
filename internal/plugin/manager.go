package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ai-flight-dashboard/internal/source"
)

const (
	maxPluginArchive  = 10 << 20
	maxPluginUnpacked = 20 << 20
	maxPluginFiles    = 200
	maxManifestSize   = 256 << 10
)

// Installed describes a plugin found on disk.
type Installed struct {
	Manifest Manifest `json:"manifest"`
	Path     string   `json:"path"`
	Enabled  bool     `json:"enabled"`
}

// Manager installs, removes, and loads declarative plugin packages.
type Manager struct {
	dir        string
	appVersion string
	enabled    func(string) bool
	mu         sync.Mutex
}

func NewManager(dataDir, appVersion string, enabled func(string) bool) *Manager {
	if enabled == nil {
		enabled = func(string) bool { return true }
	}
	return &Manager{
		dir:        filepath.Join(dataDir, "plugins"),
		appVersion: appVersion,
		enabled:    enabled,
	}
}

func (m *Manager) Dir() string { return m.dir }

func (m *Manager) IsEnabled(id string) bool { return m.enabled(id) }

func (m *Manager) List() ([]Installed, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Installed, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		manifest, err := m.readManifest(entry.Name())
		if err != nil {
			continue
		}
		if !versionAtLeast(m.appVersion, manifest.MinAppVersion) {
			continue
		}
		out = append(out, Installed{
			Manifest: manifest,
			Path:     filepath.Join(m.dir, entry.Name()),
			Enabled:  m.enabled(manifest.ID),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.Name < out[j].Manifest.Name })
	return out, nil
}

func (m *Manager) LoadSources() []source.Source {
	installed, err := m.List()
	if err != nil {
		return nil
	}
	out := make([]source.Source, 0, len(installed))
	for _, item := range installed {
		if !item.Enabled {
			continue
		}
		src, err := NewSource(item.Manifest)
		if err != nil {
			continue
		}
		out = append(out, src)
	}
	return out
}

func (m *Manager) LoadSource(id string) (source.Source, error) {
	manifest, err := m.readManifest(id)
	if err != nil {
		return nil, err
	}
	return NewSource(manifest)
}

func (m *Manager) readManifest(id string) (Manifest, error) {
	if !idPattern.MatchString(id) {
		return Manifest{}, fmt.Errorf("invalid plugin id %q", id)
	}
	data, err := os.ReadFile(filepath.Join(m.dir, id, ManifestName))
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.ID != id {
		return Manifest{}, fmt.Errorf("manifest id %q does not match directory %q", manifest.ID, id)
	}
	return manifest, nil
}

// InstallZip extracts a plugin archive and replaces any installed version of
// the same plugin. The archive may have manifest.json at its root or inside a
// single top-level directory.
func (m *Manager) InstallZip(r io.Reader) (Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxPluginArchive+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxPluginArchive {
		return Manifest{}, fmt.Errorf("plugin archive exceeds %d bytes", maxPluginArchive)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Manifest{}, fmt.Errorf("open plugin archive: %w", err)
	}

	manifestFile, rootPrefix, err := findManifest(reader)
	if err != nil {
		return Manifest{}, err
	}
	raw, err := readZipEntry(manifestFile, maxManifestSize)
	if err != nil {
		return Manifest{}, err
	}
	if len(raw) > maxManifestSize {
		return Manifest{}, fmt.Errorf("manifest exceeds %d bytes", maxManifestSize)
	}
	manifest, err := decodeManifest(raw)
	if err != nil {
		return Manifest{}, err
	}
	if !versionAtLeast(m.appVersion, manifest.MinAppVersion) {
		return Manifest{}, fmt.Errorf("plugin requires app version %s or newer", manifest.MinAppVersion)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return Manifest{}, err
	}
	tmp, err := os.MkdirTemp(m.dir, ".install-"+manifest.ID+"-")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(tmp)

	if err := extractPlugin(reader, rootPrefix, tmp); err != nil {
		return Manifest{}, err
	}
	target := filepath.Join(m.dir, manifest.ID)
	if err := replaceDir(tmp, target); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m *Manager) InstallFile(path string) (Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	return m.InstallZip(f)
}

func (m *Manager) InstallURL(ctx context.Context, rawURL, wantSHA256 string) (Manifest, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Manifest{}, fmt.Errorf("download URL must be http or https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Manifest{}, err
	}
	req.Header.Set("User-Agent", "AI-Flight-Dashboard-Plugin-Installer")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginArchive+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxPluginArchive {
		return Manifest{}, fmt.Errorf("plugin archive exceeds %d bytes", maxPluginArchive)
	}
	if wantSHA256 != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(wantSHA256)) {
			return Manifest{}, fmt.Errorf("plugin checksum mismatch")
		}
	}
	return m.InstallZip(bytes.NewReader(data))
}

func (m *Manager) Remove(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid plugin id %q", id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return os.RemoveAll(filepath.Join(m.dir, id))
}

func findManifest(reader *zip.Reader) (*zip.File, string, error) {
	var best *zip.File
	bestDepth := -1
	ambiguous := false
	for _, file := range reader.File {
		name := normalizeZipName(file.Name)
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		if filepath.Base(name) != ManifestName {
			continue
		}
		depth := strings.Count(name, "/")
		if best == nil || depth < bestDepth {
			best = file
			bestDepth = depth
			ambiguous = false
		} else if depth == bestDepth {
			ambiguous = true
		}
	}
	if best == nil {
		return nil, "", fmt.Errorf("plugin archive is missing %s", ManifestName)
	}
	if ambiguous {
		return nil, "", fmt.Errorf("plugin archive contains multiple %s files", ManifestName)
	}
	name := normalizeZipName(best.Name)
	root := ""
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		root = name[:idx+1]
	}
	return best, root, nil
}

func extractPlugin(reader *zip.Reader, rootPrefix, dest string) error {
	var total int64
	count := 0
	for _, file := range reader.File {
		name := normalizeZipName(file.Name)
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		if rootPrefix != "" && !strings.HasPrefix(name, rootPrefix) {
			continue
		}
		rel := strings.TrimPrefix(name, rootPrefix)
		if rel == "" {
			continue
		}
		clean := filepath.Clean(rel)
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in plugin archive: %q", file.Name)
		}
		count++
		if count > maxPluginFiles {
			return fmt.Errorf("plugin archive contains too many files")
		}
		if int64(file.UncompressedSize64) > maxPluginUnpacked {
			return fmt.Errorf("plugin archive expands beyond %d bytes", maxPluginUnpacked)
		}
		target := filepath.Join(dest, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		remaining := maxPluginUnpacked - total
		if remaining <= 0 {
			return fmt.Errorf("plugin archive expands beyond %d bytes", maxPluginUnpacked)
		}
		data, err := readZipEntry(file, remaining)
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > maxPluginUnpacked {
			return fmt.Errorf("plugin archive expands beyond %d bytes", maxPluginUnpacked)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dest, ManifestName)); err != nil {
		return fmt.Errorf("plugin archive did not contain %s at its root", ManifestName)
	}
	return nil
}

func readZipEntry(file *zip.File, limit int64) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit+1))
}

func normalizeZipName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimPrefix(name, "./")
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return ""
	}
	return name
}

func replaceDir(tmp, target string) error {
	backup := filepath.Join(filepath.Dir(target), ".old-"+filepath.Base(target)+"-"+fmt.Sprint(time.Now().UnixNano()))
	hadOld := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		hadOld = true
	}
	if err := os.Rename(tmp, target); err != nil {
		if hadOld {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if hadOld {
		_ = os.RemoveAll(backup)
	}
	return nil
}
