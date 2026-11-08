package main

import (
	goruntime "runtime"

	"ai-flight-dashboard/internal/desktop"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func configureGUIWindowLifecycle(appOptions *options.App, app *desktop.App) {
	// Windows and macOS have a working tray/menu-bar icon to restore the
	// window. Linux status-notifier support varies by desktop, so closing the
	// window quits instead of leaving an unreachable background process.
	appOptions.HideWindowOnClose = goruntime.GOOS != "linux"
	appOptions.SingleInstanceLock = &options.SingleInstanceLock{
		UniqueId: "ai-flight-dashboard",
		OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
			app.ShowWindow()
		},
	}
}
