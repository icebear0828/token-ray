package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// Default repository
var (
	GitHubRepo    = "icebear0828/token-ray"
	GitHubAPIBase = "https://api.github.com"
)

type Asset struct {
	Name               string `json:"name"`
	URL                string `json:"url"` // API download URL
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

type UpdateStatus struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	PublishedAt    string `json:"published_at"`
	DownloadURL    string `json:"download_url"`
	AssetName      string `json:"asset_name"`
	Error          string `json:"error,omitempty"`
}

// Client configures update check and apply operations
type Client struct {
	BaseURL    string
	Repo       string
	Token      string
	HTTPClient *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.BaseURL = strings.TrimRight(url, "/")
	}
}

func WithRepo(repo string) Option {
	return func(c *Client) {
		c.Repo = repo
	}
}

func WithToken(token string) Option {
	return func(c *Client) {
		c.Token = token
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.HTTPClient = client
	}
}

func NewClient(opts ...Option) *Client {
	c := &Client{
		BaseURL:    GitHubAPIBase,
		Repo:       GitHubRepo,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// parseSemver parses "1.2.3" into [1, 2, 3].
func parseSemver(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	res := make([]int, 0, len(parts))
	for _, p := range parts {
		// Handle potential suffix like 1.2.3-beta
		subParts := strings.Split(p, "-")
		if n, err := strconv.Atoi(subParts[0]); err == nil {
			res = append(res, n)
		} else {
			res = append(res, 0)
		}
	}
	for len(res) < 3 {
		res = append(res, 0)
	}
	return res
}

// IsNewerVersion returns true if remote is strictly greater than local.
func IsNewerVersion(local, remote string) bool {
	localNorm := strings.TrimPrefix(strings.TrimSpace(local), "v")
	remoteNorm := strings.TrimPrefix(strings.TrimSpace(remote), "v")

	if remoteNorm == "" {
		return false
	}
	if localNorm == "" || localNorm == "dev" || localNorm == "0.0.0" {
		return true
	}

	lParts := parseSemver(localNorm)
	rParts := parseSemver(remoteNorm)

	for i := 0; i < 3; i++ {
		if rParts[i] > lParts[i] {
			return true
		}
		if rParts[i] < lParts[i] {
			return false
		}
	}
	return false
}

// CheckForUpdates queries the latest release and checks if a newer version exists.
func (c *Client) CheckForUpdates(currentVersion string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.BaseURL, c.Repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to check for updates: HTTP %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode release response: %w", err)
	}

	if IsNewerVersion(currentVersion, rel.TagName) {
		return &rel, nil
	}

	return nil, nil
}

// nonBinaryAssetSuffixes lists release assets that are never runnable binaries,
// so they must not be selected as a self-update payload.
var nonBinaryAssetSuffixes = []string{
	".sha256", ".sha512", ".sha256sum", ".sha512sum", ".md5",
	".sig", ".asc", ".pem", ".crt", ".cert",
	".txt", ".md", ".json", ".sbom", ".spdx",
	".deb", ".rpm", ".msi", ".pkg", ".dmg",
}

func isArchiveName(name string) bool {
	return strings.HasSuffix(name, ".tar.gz") ||
		strings.HasSuffix(name, ".tgz") ||
		strings.HasSuffix(name, ".zip")
}

func isBinaryAssetName(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range nonBinaryAssetSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	return true
}

// MatchAsset finds the most appropriate release asset for the target OS and Architecture.
func MatchAsset(assets []Asset, goos, goarch string) *Asset {
	// 1. First priority: a raw binary for the exact target platform.
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if isArchiveName(name) || !isBinaryAssetName(name) {
			continue
		}
		if strings.Contains(name, goos) && strings.Contains(name, goarch) {
			return &assets[i]
		}
	}

	// 2. Second priority: archives published by the release workflow.
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if !isBinaryAssetName(name) {
			continue
		}
		switch {
		case goos == "darwin" && goarch == "arm64":
			if strings.Contains(name, "macos-m-series") || (strings.Contains(name, "darwin") && strings.Contains(name, "arm64")) {
				return &assets[i]
			}
		case goos == "darwin" && goarch == "amd64":
			if strings.Contains(name, "macos-intel") || (strings.Contains(name, "darwin") && strings.Contains(name, "amd64")) {
				return &assets[i]
			}
		case goos == "linux" && goarch == "amd64":
			if strings.Contains(name, "linux") && (strings.Contains(name, "amd64") || strings.HasSuffix(name, "linux.tar.gz")) {
				return &assets[i]
			}
		case goos == "windows" && goarch == "amd64":
			if strings.Contains(name, "windows") && (isArchiveName(name) || strings.HasSuffix(name, ".exe")) {
				return &assets[i]
			}
		}
	}

	return nil
}

// scoreExecutableEntry ranks archive entries so the main binary wins over
// helpers, uninstallers, or unrelated files with a similar name.
func scoreExecutableEntry(lowerName string) int {
	base := path.Base(lowerName)
	score := 0
	switch {
	case base == "ai-flight-dashboard" || base == "ai-flight-dashboard.exe":
		score += 100
	case strings.HasSuffix(base, "dashboard"):
		score += 20
	}
	if strings.Contains(lowerName, "contents/macos/") {
		score += 50
	}
	if strings.HasSuffix(base, ".exe") {
		score += 10
	}
	for _, bad := range []string{"helper", "crashpad", "uninstall", "installer", "updater"} {
		if strings.Contains(base, bad) {
			score -= 100
		}
	}
	return score
}

// ExtractBinaryFromArchive unpacks .tar.gz or .zip in-memory to find the executable binary.
func ExtractBinaryFromArchive(r io.Reader, assetName string) (io.Reader, error) {
	lowerName := strings.ToLower(assetName)

	if strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz") {
		gzr, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("gzip unpack failed: %w", err)
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		var bestMatch bytes.Buffer
		bestScore := 0

		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("tar read error: %w", err)
			}
			if hdr.Typeflag != tar.TypeReg {
				continue
			}

			score := scoreExecutableEntry(strings.ToLower(hdr.Name))
			if score <= bestScore {
				continue
			}
			bestMatch.Reset()
			if _, err := io.Copy(&bestMatch, tr); err != nil {
				return nil, fmt.Errorf("failed to read tar entry %s: %w", hdr.Name, err)
			}
			bestScore = score
		}

		if bestScore <= 0 {
			return nil, fmt.Errorf("no executable binary found inside archive %s", assetName)
		}
		return bytes.NewReader(bestMatch.Bytes()), nil
	}

	if strings.HasSuffix(lowerName, ".zip") {
		// Read entire zip into buffer for zip.Reader
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read zip buffer: %w", err)
		}

		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("zip reader failed: %w", err)
		}

		var best *zip.File
		bestScore := 0
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			score := scoreExecutableEntry(strings.ToLower(f.Name))
			if score > bestScore {
				best = f
				bestScore = score
			}
		}
		if best == nil {
			return nil, fmt.Errorf("no executable binary found inside zip %s", assetName)
		}

		rc, err := best.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open zip file %s: %w", best.Name, err)
		}
		defer rc.Close()

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			return nil, fmt.Errorf("failed to copy zip file %s: %w", best.Name, err)
		}
		return bytes.NewReader(buf.Bytes()), nil
	}

	// Raw binary: return as is
	return r, nil
}

// looksLikeExecutable rejects payloads that are clearly not runnable on any
// supported platform (e.g. a checksum file matched by mistake).
func looksLikeExecutable(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return true // ELF (linux)
	}
	if data[0] == 'M' && data[1] == 'Z' {
		return true // PE (windows)
	}
	switch binary.BigEndian.Uint32(data[:4]) {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, // Mach-O
		0xcafebabe, 0xbebafeca, 0xcafebabf, 0xbfbafeca: // Mach-O universal
		return true
	}
	return false
}

// ApplyUpdate downloads the platform asset and applies it in-place using selfupdate.
func (c *Client) ApplyUpdate(release *Release) error {
	asset := MatchAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		return fmt.Errorf("no suitable asset found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	downloadURL := asset.URL
	if downloadURL == "" {
		downloadURL = asset.BrowserDownloadURL
	}
	if downloadURL == "" {
		return fmt.Errorf("no download URL for asset %s", asset.Name)
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create asset download request: %w", err)
	}

	req.Header.Set("Accept", "application/octet-stream")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	downloadClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code downloading update asset: %d", resp.StatusCode)
	}

	// Extract binary stream if it's an archive
	binaryReader, err := ExtractBinaryFromArchive(resp.Body, asset.Name)
	if err != nil {
		return fmt.Errorf("failed to extract binary from archive: %w", err)
	}

	const maxBinarySize = 256 << 20
	data, err := io.ReadAll(io.LimitReader(binaryReader, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("failed to read extracted binary: %w", err)
	}
	if int64(len(data)) > maxBinarySize {
		return fmt.Errorf("extracted binary exceeds %d bytes", maxBinarySize)
	}
	if !looksLikeExecutable(data) {
		return fmt.Errorf("downloaded asset %s does not look like a runnable binary", asset.Name)
	}

	// In-place binary replacement
	if err := selfupdate.Apply(bytes.NewReader(data), selfupdate.Options{}); err != nil {
		return fmt.Errorf("selfupdate failed: %w", err)
	}

	return nil
}

// Global helper wrappers (backward compatibility)

func CheckForUpdates(currentVersion, token string) (*Release, error) {
	client := NewClient(WithToken(token))
	return client.CheckForUpdates(currentVersion)
}

func ApplyUpdate(release *Release, token string) error {
	client := NewClient(WithToken(token))
	return client.ApplyUpdate(release)
}
