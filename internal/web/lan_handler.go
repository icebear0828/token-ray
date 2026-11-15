package web

import (
	"encoding/json"
	"net/http"

	"token-ray/internal/dashboard"
	"token-ray/internal/db"
	"token-ray/internal/lan"
	"token-ray/internal/model"
)

// NewLANHandler exposes only the endpoints needed by LAN peers.
func NewLANHandler(database *db.DB, token string, lanInst *lan.LAN) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/lan/self", func(w http.ResponseWriter, r *http.Request) {
		handleLANSelf(w, r, lanInst)
	})
	mux.HandleFunc("/api/sync/pull", syncAuthMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handleSyncPull(w, r, database)
	}))
	return mux
}

func handleLANSelf(w http.ResponseWriter, r *http.Request, lanInst *lan.LAN) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if lanInst == nil {
		http.NotFound(w, r)
		return
	}

	resp := model.LANSelfResponse{
		DeviceID: lanInst.DeviceID,
		HTTPPort: lanInst.HTTPPort,
	}
	if summary, ok := lanInst.CurrentSummary(); ok {
		resp.Summary = &summary
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func currentLAN(lanControl LANController) *lan.LAN {
	if lanControl == nil {
		return nil
	}
	return lanControl.CurrentLAN()
}

func handleLANScan(w http.ResponseWriter, r *http.Request, database *db.DB, lanInst *lan.LAN) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	peers := make([]string, 0)
	peerInfos := make([]model.LANPeerInfo, 0)
	if lanInst != nil {
		peers = lanInst.GetActivePeers()
		aliases, _ := database.GetDeviceAliases()
		for _, peer := range lanInst.GetActivePeerInfos() {
			summary, err := dashboard.BuildTokenSummary(database, peer.ID)
			if err != nil {
				summary = model.TokenSummary{}
			}
			if peer.HasSummary && (peer.SyncStatus != "ok" || summary.TokensTotal == 0) {
				summary = peer.Summary
			}
			displayName := peer.ID
			if alias := aliases[peer.ID]; alias != "" {
				displayName = alias
			}
			peerInfos = append(peerInfos, model.LANPeerInfo{
				ID:              peer.ID,
				DisplayName:     displayName,
				IP:              peer.IP,
				HTTPPort:        peer.HTTPPort,
				LastSeen:        peer.LastSeen,
				LastSync:        peer.LastSync,
				LastSyncAttempt: peer.LastSyncAttempt,
				SyncStatus:      peer.SyncStatus,
				SyncError:       peer.SyncError,
				Tokens24h:       summary.Tokens24h,
				TokensTotal:     summary.TokensTotal,
				CostTotal:       summary.CostTotal,
				Sources:         summary.Sources,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.LANScanResponse{Peers: peers, PeerInfos: peerInfos})
}

func handleLANStatus(w http.ResponseWriter, r *http.Request, lanControl LANController) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := model.LANStatusResponse{Enabled: false}
	if lanControl != nil {
		status = lanControl.Status()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func handleLANJoin(w http.ResponseWriter, r *http.Request, lanControl LANController) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := model.LANStatusResponse{Enabled: false}
	var err error
	if lanControl != nil {
		status, err = lanControl.Join()
	}
	if err != nil {
		http.Error(w, "Failed to join LAN", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func handleLANLeave(w http.ResponseWriter, r *http.Request, lanControl LANController) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := model.LANStatusResponse{Enabled: false}
	var err error
	if lanControl != nil {
		status, err = lanControl.Leave()
	}
	if err != nil {
		http.Error(w, "Failed to leave LAN", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
