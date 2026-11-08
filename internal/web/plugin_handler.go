package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ai-flight-dashboard/internal/plugin"
	"ai-flight-dashboard/internal/source"
)

// SourceProvider exposes the active sources to the UI.
type SourceProvider interface {
	Descriptors() []source.Descriptor
}

// PluginController is the plugin management surface used by the HTTP API.
type PluginController interface {
	List() ([]plugin.Installed, error)
	InstallZip(r io.Reader) (plugin.Manifest, error)
	InstallURL(ctx context.Context, rawURL, sha256 string) (plugin.Manifest, error)
	Remove(id string) error
	SetEnabled(id string, enabled bool) error
	FetchIndex(ctx context.Context) ([]byte, error)
}

func handleSources(w http.ResponseWriter, r *http.Request, provider SourceProvider) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var descriptors []source.Descriptor
	if provider != nil {
		descriptors = provider.Descriptors()
	}
	if descriptors == nil {
		descriptors = []source.Descriptor{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(descriptors)
}

func handlePlugins(w http.ResponseWriter, r *http.Request, ctl PluginController) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ctl == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]plugin.Installed{})
		return
	}
	installed, err := ctl.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if installed == nil {
		installed = []plugin.Installed{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(installed)
}

func handlePluginInstall(w http.ResponseWriter, r *http.Request, ctl PluginController) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ctl == nil {
		http.Error(w, "Plugins are not available", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPluginUpload)

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxPluginUpload); err != nil {
			http.Error(w, "Invalid upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing plugin file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		manifest, err := ctl.InstallZip(file)
		writePluginResult(w, manifest, err)
		return
	}

	var req struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	manifest, err := ctl.InstallURL(r.Context(), req.URL, req.SHA256)
	writePluginResult(w, manifest, err)
}

func handlePluginIndex(w http.ResponseWriter, r *http.Request, ctl PluginController) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ctl == nil {
		http.Error(w, "Plugins are not available", http.StatusServiceUnavailable)
		return
	}
	data, err := ctl.FetchIndex(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func handlePluginAction(w http.ResponseWriter, r *http.Request, ctl PluginController) {
	if ctl == nil {
		http.Error(w, "Plugins are not available", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	id, action, _ := strings.Cut(path, "/")
	if id == "" {
		http.Error(w, "Missing plugin id", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodDelete && action == "":
		if err := ctl.Remove(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && (action == "enable" || action == "disable"):
		if err := ctl.SetEnabled(id, action == "enable"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func writePluginResult(w http.ResponseWriter, manifest plugin.Manifest, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(manifest)
}
