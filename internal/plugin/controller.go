package plugin

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/source"
)

// DirWatcher is the subset of watcher.Watcher used to keep filesystem
// registrations in sync with enabled plugins.
type DirWatcher interface {
	WatchDirRecursive(dir string) error
	UnwatchDir(dir string) error
}

// Controller ties the plugin manager to the live source registry so plugins
// can be installed, enabled, and removed without restarting the app.
type Controller struct {
	manager  *Manager
	registry *source.Registry
	watcher  DirWatcher
	sink     source.Sink
	mu       sync.Mutex
}

func NewController(manager *Manager, registry *source.Registry, watcher DirWatcher, sink source.Sink) *Controller {
	return &Controller{manager: manager, registry: registry, watcher: watcher, sink: sink}
}

func (c *Controller) List() ([]Installed, error) { return c.manager.List() }

func (c *Controller) InstallZip(r io.Reader) (Manifest, error) {
	manifest, err := c.manager.InstallZip(r)
	if err != nil {
		return Manifest{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unregister(manifest.ID)
	if c.manager.IsEnabled(manifest.ID) {
		if err := c.register(manifest.ID); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

func (c *Controller) InstallURL(ctx context.Context, rawURL, sha256 string) (Manifest, error) {
	manifest, err := c.manager.InstallURL(ctx, rawURL, sha256)
	if err != nil {
		return Manifest{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unregister(manifest.ID)
	if c.manager.IsEnabled(manifest.ID) {
		if err := c.register(manifest.ID); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

func (c *Controller) Remove(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unregister(id)
	if err := c.manager.Remove(id); err != nil {
		return err
	}
	return setPluginDisabled(id, false)
}

func (c *Controller) SetEnabled(id string, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.manager.readManifest(id); err != nil {
		return err
	}
	if enabled {
		if err := c.register(id); err != nil {
			return err
		}
	} else {
		c.unregister(id)
	}
	return setPluginDisabled(id, !enabled)
}

func (c *Controller) FetchIndex(ctx context.Context) ([]byte, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	rawURL := strings.TrimSpace(cfg.PluginIndexURL)
	if rawURL == "" {
		return nil, fmt.Errorf("plugin index URL is not configured")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("plugin index URL must be http or https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin index returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Controller) register(id string) error {
	src, err := c.manager.LoadSource(id)
	if err != nil {
		return err
	}
	c.registry.Register(src)
	dirs := src.WatchDirs()
	log.Printf("plugin %s registered; watch dirs: %v", id, dirs)
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil || c.watcher == nil {
			continue
		}
		_ = c.watcher.WatchDirRecursive(dir)
	}
	if c.sink != nil {
		go func() {
			if err := src.Scan(context.Background(), c.sink); err != nil {
				log.Printf("plugin %s initial scan failed: %v", id, err)
			}
		}()
	}
	return nil
}

func (c *Controller) unregister(id string) {
	src := c.registry.Get(id)
	if src == nil {
		return
	}
	for _, dir := range src.WatchDirs() {
		if c.watcher != nil {
			_ = c.watcher.UnwatchDir(dir)
		}
	}
	c.registry.Unregister(id)
}

func setPluginDisabled(id string, disabled bool) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	out := make([]string, 0, len(cfg.DisabledPlugins)+1)
	for _, existing := range cfg.DisabledPlugins {
		if existing != id {
			out = append(out, existing)
		}
	}
	if disabled {
		out = append(out, id)
	}
	cfg.DisabledPlugins = out
	return config.SaveConfig(cfg)
}
