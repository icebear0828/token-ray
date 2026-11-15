package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"token-ray/internal/config"
)

func TestLoadConfig_FileNotExist(t *testing.T) {
	// Use a temp dir so we don't read any real config
	tmpDir := t.TempDir()
	config.SetDataDir(tmpDir)
	defer config.SetDataDir("")

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should not error when file missing, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned nil config")
	}
	if cfg.AutoStart {
		t.Error("default AutoStart should be false")
	}
	if len(cfg.ExtraWatchDirs) != 0 {
		t.Errorf("default ExtraWatchDirs should be empty, got %v", cfg.ExtraWatchDirs)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetDataDir(tmpDir)
	defer config.SetDataDir("")

	cfg := &config.AppConfig{
		AutoStart:      true,
		ExtraWatchDirs: []string{"/tmp/logs", "/var/data"},
	}

	err := config.SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	loaded, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !loaded.AutoStart {
		t.Error("AutoStart should be true after save/load")
	}
	if len(loaded.ExtraWatchDirs) != 2 {
		t.Errorf("expected 2 ExtraWatchDirs, got %d", len(loaded.ExtraWatchDirs))
	}
	if loaded.ExtraWatchDirs[0] != "/tmp/logs" {
		t.Errorf("expected /tmp/logs, got %s", loaded.ExtraWatchDirs[0])
	}
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetDataDir(tmpDir)
	defer config.SetDataDir("")

	os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{invalid json`), 0644)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should not error on bad JSON, got: %v", err)
	}
	if cfg.AutoStart {
		t.Error("malformed JSON should result in default (false) AutoStart")
	}
}

func TestGetConfigPath(t *testing.T) {
	config.SetDataDir("")
	t.Setenv("TOKEN_RAY_DATA_DIR", "")

	path := config.GetConfigPath()
	if filepath.Base(path) != "config.json" {
		t.Errorf("expected config.json, got %s", filepath.Base(path))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expectedDefault := filepath.Join(home, ".token-ray", "config.json")
	if path != expectedDefault {
		t.Errorf("expected default config path %s, got %s", expectedDefault, path)
	}

	// Custom dir
	config.SetDataDir("/custom/path")
	path = config.GetConfigPath()
	if path != filepath.Join("/custom/path", "config.json") {
		t.Errorf("expected /custom/path/config.json, got %s", path)
	}
	config.SetDataDir("")
}

func TestGetCustomPricingPathUsesDataDir(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	path := config.GetCustomPricingPath()
	expected := filepath.Join(dataDir, "custom_pricing.json")
	if path != expected {
		t.Errorf("expected custom pricing path %s, got %s", expected, path)
	}
}

func TestGetDataDir_Default(t *testing.T) {
	config.SetDataDir("")
	t.Setenv("TOKEN_RAY_DATA_DIR", "")

	dir := config.GetDataDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, ".token-ray")
	if dir != expected {
		t.Errorf("expected default home data directory %s, got %s", expected, dir)
	}
}

func TestGetDataDir_Custom(t *testing.T) {
	config.SetDataDir("/my/data")
	defer config.SetDataDir("")
	dir := config.GetDataDir()
	expected := filepath.Clean("/my/data")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestGetDataDir_EnvironmentOverride(t *testing.T) {
	config.SetDataDir("")
	envDir := filepath.Join(t.TempDir(), "env-data")
	t.Setenv("TOKEN_RAY_DATA_DIR", envDir)

	dir := config.GetDataDir()
	if dir != envDir {
		t.Errorf("expected environment data directory %s, got %s", envDir, dir)
	}
}

func TestGetDataDir_CustomOverridesEnvironment(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "env-data")
	customDir := filepath.Join(t.TempDir(), "custom-data")
	t.Setenv("TOKEN_RAY_DATA_DIR", envDir)
	config.SetDataDir(customDir)
	defer config.SetDataDir("")

	dir := config.GetDataDir()
	if dir != customDir {
		t.Errorf("expected custom data directory %s, got %s", customDir, dir)
	}
}

func TestGetDataDir_ExpandsHomeDirectory(t *testing.T) {
	config.SetDataDir("~/token-ray-test")
	defer config.SetDataDir("")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, "token-ray-test")
	if dir := config.GetDataDir(); dir != expected {
		t.Errorf("expected expanded home data directory %s, got %s", expected, dir)
	}
}
