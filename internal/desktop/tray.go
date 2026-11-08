package desktop

import (
	"sync"

	"github.com/energye/systray"
	"github.com/energye/systray/icon"
)

// Tray owns the notification-area / menu-bar icon. Menu callbacks run on the
// systray event loop, so they only use thread-safe Wails runtime calls.
type Tray struct {
	app *App

	mu      sync.Mutex
	started bool
	end     func()
}

func NewTray(app *App) *Tray {
	return &Tray{app: app}
}

// Start registers the tray icon and starts its event loop. It is a no-op if
// the tray is already running.
func (t *Tray) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return
	}
	t.started = true
	start, end := systray.RunWithExternalLoop(t.onReady, func() {})
	t.end = end
	start()
}

// Stop removes the tray icon. It is safe to call more than once.
func (t *Tray) Stop() {
	t.mu.Lock()
	end := t.end
	t.end = nil
	t.started = false
	t.mu.Unlock()
	if end != nil {
		end()
	}
}

func (t *Tray) onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTitle("AI Flight Dashboard")
	systray.SetTooltip("AI Flight Dashboard")

	show := systray.AddMenuItem("Show Dashboard", "Open the dashboard window")
	quit := systray.AddMenuItem("Quit", "Quit AI Flight Dashboard")
	show.Click(func() { t.app.ShowWindow() })
	quit.Click(func() { t.app.Quit() })

	// Left-click restores the window; right-click opens the menu.
	systray.SetOnClick(func(systray.IMenu) { t.app.ShowWindow() })
	systray.SetOnDClick(func(systray.IMenu) { t.app.ShowWindow() })
	systray.SetOnRClick(func(menu systray.IMenu) { _ = menu.ShowMenu() })
}
