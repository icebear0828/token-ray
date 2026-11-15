package desktop

import (
	"strings"
	"testing"
)

func TestDarwinAutoStartPlistUsesExplicitDataDir(t *testing.T) {
	content := darwinAutoStartPlist("/Applications/Token Ray.app/Contents/MacOS/token-ray", "/Users/c/.token-ray")

	if strings.Contains(content, "--gui") {
		t.Fatalf("autostart plist must not include unsupported --gui flag: %s", content)
	}
	for _, want := range []string{
		"<string>--data-dir</string>",
		"<string>/Users/c/.token-ray</string>",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("autostart plist missing %q: %s", want, content)
		}
	}
}

func TestLinuxAutoStartDesktopEntryUsesExplicitDataDir(t *testing.T) {
	content := linuxAutoStartDesktopEntry("/usr/local/bin/token-ray", "/home/c/.token-ray")

	if strings.Contains(content, "--gui") {
		t.Fatalf("autostart desktop entry must not include unsupported --gui flag: %s", content)
	}
	if !strings.Contains(content, "Exec=/usr/local/bin/token-ray --data-dir /home/c/.token-ray") {
		t.Fatalf("autostart desktop entry missing explicit data-dir: %s", content)
	}
}
