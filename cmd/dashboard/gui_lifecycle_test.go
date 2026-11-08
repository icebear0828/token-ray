package main

import (
	goruntime "runtime"
	"testing"

	"ai-flight-dashboard/internal/desktop"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestConfigureGUIWindowLifecycleHidesWindowOnClose(t *testing.T) {
	appOptions := &options.App{}
	app := desktop.NewApp(nil, nil)

	configureGUIWindowLifecycle(appOptions, app)

	expected := goruntime.GOOS != "linux"
	if appOptions.HideWindowOnClose != expected {
		t.Fatalf("expected HideWindowOnClose=%v on %s", expected, goruntime.GOOS)
	}
}

func TestConfigureGUIWindowLifecycleRestoresHiddenWindowOnSecondLaunch(t *testing.T) {
	appOptions := &options.App{}
	app := desktop.NewApp(nil, nil)

	configureGUIWindowLifecycle(appOptions, app)

	if appOptions.SingleInstanceLock == nil {
		t.Fatal("expected single-instance lock to be configured")
	}
	if appOptions.SingleInstanceLock.UniqueId != "ai-flight-dashboard" {
		t.Fatalf("unexpected single-instance ID: %q", appOptions.SingleInstanceLock.UniqueId)
	}
	if appOptions.SingleInstanceLock.OnSecondInstanceLaunch == nil {
		t.Fatal("expected second-launch callback to restore hidden window")
	}
}
