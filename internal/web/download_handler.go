package web

import (
	"embed"
	"fmt"
	"net/http"
	"os"
)

func handleDownload(w http.ResponseWriter, r *http.Request, distBinFS embed.FS) {
	filename := r.URL.Path[len("/download/"):]
	if filename == "dashboard" || filename == "" {
		exePath, err := os.Executable()
		if err == nil {
			w.Header().Set("Content-Disposition", "attachment; filename=dashboard")
			http.ServeFile(w, r, exePath)
			return
		}
	}

	data, err := distBinFS.ReadFile("dist-bin/" + filename)
	if err != nil {
		exePath, err2 := os.Executable()
		if err2 == nil {
			w.Header().Set("Content-Disposition", "attachment; filename=dashboard")
			http.ServeFile(w, r, exePath)
			return
		}
		http.Error(w, "Binary not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Write(data)
}

func handleInstallScript(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = "localhost:19100"
	}
	script := fmt.Sprintf("#!/bin/bash\n"+
		"OS=$(uname -s | tr '[:upper:]' '[:lower:]')\n"+
		"ARCH=$(uname -m)\n"+
		"if [ \"$ARCH\" = \"x86_64\" ]; then ARCH=\"amd64\"; fi\n"+
		"if [ \"$ARCH\" = \"aarch64\" ]; then ARCH=\"arm64\"; fi\n"+
		"echo \"📡 Downloading Token Ray ($OS-$ARCH) from %s...\"\n"+
		"curl -o dashboard http://%s/download/dashboard-$OS-$ARCH\n"+
		"chmod +x dashboard\n"+
		"echo \"✅ Download complete! Starting LAN mode...\"\n"+
		"./dashboard --lan\n", host, host)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(script))
}
