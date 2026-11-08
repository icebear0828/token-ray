package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	AutoStart          bool     `json:"auto_start"`
	ExtraWatchDirs     []string `json:"extra_watch_dirs"`
	EnableLAN          *bool    `json:"enable_lan,omitempty"`
	ExtraPeerHosts     []string `json:"extra_peer_hosts,omitempty"`
	TailscaleDiscovery *bool    `json:"tailscale_discovery,omitempty"`
	DisabledPlugins    []string `json:"disabled_plugins,omitempty"`
	PluginIndexURL     string   `json:"plugin_index_url,omitempty"`
}

// customDir allows overriding the data directory at runtime.
var customDir string

const DataDirEnv = "AI_FLIGHT_DASHBOARD_DATA_DIR"

// SetDataDir overrides the default ~/.ai-flight-dashboard data directory.
func SetDataDir(dir string) {
	customDir = dir
}

// GetDataDir returns the application data directory.
func GetDataDir() string {
	if customDir != "" {
		return normalizeDataDir(customDir)
	}
	if envDir := strings.TrimSpace(os.Getenv(DataDirEnv)); envDir != "" {
		return normalizeDataDir(envDir)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dir := filepath.Join(home, ".ai-flight-dashboard")
		os.MkdirAll(dir, 0755)
		return dir
	}
	return "."
}

func normalizeDataDir(dir string) string {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return trimmed
	}
	if strings.HasPrefix(trimmed, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(trimmed, "~/"))
		}
	}
	return filepath.Clean(trimmed)
}

func GetConfigPath() string {
	return filepath.Join(GetDataDir(), "config.json")
}

func GetCustomPricingPath() string {
	return filepath.Join(GetDataDir(), "custom_pricing.json")
}

func LoadConfig() (*AppConfig, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		t := true
		return &AppConfig{EnableLAN: &t}, nil
	}
	cfg := &AppConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		t := true
		return &AppConfig{EnableLAN: &t}, nil
	}

	if cfg.EnableLAN == nil {
		t := true
		cfg.EnableLAN = &t
	}

	return cfg, nil
}

func SaveConfig(cfg *AppConfig) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
