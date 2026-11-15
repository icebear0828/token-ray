package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"token-ray/internal/config"
)

// SetAutoStart registers or unregisters the application for auto-start on login.
func SetAutoStart(enabled bool) error {
	switch runtime.GOOS {
	case "darwin":
		return setAutoStartDarwin(enabled)
	case "linux":
		return setAutoStartLinux(enabled)
	case "windows":
		return setAutoStartWindows(enabled)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// IsAutoStartEnabled checks if auto-start is currently configured.
func IsAutoStartEnabled() bool {
	switch runtime.GOOS {
	case "darwin":
		plist := launchAgentPath()
		_, err := os.Stat(plist)
		return err == nil
	case "linux":
		desktop := autostartDesktopPath()
		_, err := os.Stat(desktop)
		return err == nil
	default:
		return false
	}
}

// --- macOS: LaunchAgent plist ---

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.token-ray.plist")
}

func setAutoStartDarwin(enabled bool) error {
	plistPath := launchAgentPath()

	if !enabled {
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	plistContent := darwinAutoStartPlist(exePath, config.GetDataDir())

	dir := filepath.Dir(plistPath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(plistPath, []byte(plistContent), 0644)
}

func darwinAutoStartPlist(exePath string, dataDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.token-ray</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--data-dir</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`, exePath, dataDir)
}

// --- Linux: XDG Autostart ---

func autostartDesktopPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "token-ray.desktop")
}

func setAutoStartLinux(enabled bool) error {
	desktopPath := autostartDesktopPath()

	if !enabled {
		if err := os.Remove(desktopPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	content := linuxAutoStartDesktopEntry(exePath, config.GetDataDir())

	dir := filepath.Dir(desktopPath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(desktopPath, []byte(content), 0644)
}

func linuxAutoStartDesktopEntry(exePath string, dataDir string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Token Ray
Exec=%s --data-dir %s
Icon=token-ray
Terminal=false
X-GNOME-Autostart-enabled=true
`, exePath, dataDir)
}

// --- Windows: Registry (stub — requires golang.org/x/sys/windows) ---

func setAutoStartWindows(enabled bool) error {
	// Windows autostart via registry requires the golang.org/x/sys/windows package.
	// For now, this is a placeholder — will be implemented when Windows CI is set up.
	return fmt.Errorf("windows autostart: not yet implemented (requires registry access)")
}
