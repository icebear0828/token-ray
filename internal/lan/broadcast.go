package lan

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"token-ray/internal/model"
)

// StartPinger periodically sends ping packets to the LAN.
func (l *LAN) StartPinger() {
	l.StartPingerContext(context.Background())
}

// StartPingerContext periodically sends ping packets until ctx is canceled.
func (l *LAN) StartPingerContext(ctx context.Context) {
	if data, err := json.Marshal(l.newPayload("ping")); err == nil {
		l.sendToAll(data)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if data, err := json.Marshal(l.newPayload("ping")); err == nil {
				l.sendToAll(data)
			}
		}
	}
}

// Ping sends a single ping packet to announce presence (for ad-hoc use).
func (l *LAN) Ping() {
	data, err := json.Marshal(l.newPayload("ping"))
	if err == nil {
		l.sendToAll(data)
	}
}

// AnnounceDirty tells peers that local database content changed. Token data is
// still pulled over HTTP; UDP is only a wake-up signal.
func (l *LAN) AnnounceDirty() {
	l.mu.Lock()
	l.cachedSummaryOK = false
	if time.Since(l.lastDirtySent) < 2*time.Second {
		l.mu.Unlock()
		return
	}
	l.lastDirtySent = time.Now()
	l.mu.Unlock()

	data, err := json.Marshal(l.newPayload("dirty"))
	if err == nil {
		l.sendToAll(data)
	}
}

func getBroadcastAddresses() []string {
	var addrs []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{BroadcastAddr}
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrsList, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrsList {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ipnet.IP.To4() != nil {
					ip := ipnet.IP.To4()
					mask := ipnet.Mask
					if len(mask) == 4 {
						bcast := net.IPv4(
							ip[0]|^mask[0],
							ip[1]|^mask[1],
							ip[2]|^mask[2],
							ip[3]|^mask[3],
						)
						addrs = append(addrs, bcast.String()+":9101")
					}
				}
			}
		}
	}

	if len(addrs) == 0 {
		addrs = append(addrs, BroadcastAddr)
	}
	return addrs
}

func (l *LAN) sendToAll(data []byte) {
	mAddr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err == nil {
		if mConn, err := net.DialUDP("udp", nil, mAddr); err == nil {
			mConn.Write(data)
			mConn.Close()
		}
	}

	for _, bcastStr := range getBroadcastAddresses() {
		bAddr, err := net.ResolveUDPAddr("udp", bcastStr)
		if err == nil {
			if bConn, err := net.DialUDP("udp", nil, bAddr); err == nil {
				bConn.Write(data)
				bConn.Close()
			}
		}
	}

	gAddr, err := net.ResolveUDPAddr("udp", BroadcastAddr)
	if err == nil {
		if gConn, err := net.DialUDP("udp", nil, gAddr); err == nil {
			gConn.Write(data)
			gConn.Close()
		}
	}
}

// StartBroadcaster listens to local usage events and broadcasts dirty signals.
// Token records are synchronized over HTTP, not UDP.
func (l *LAN) StartBroadcaster(usageChan <-chan model.TokenUsage) {
	l.StartBroadcasterContext(context.Background(), usageChan)
}

// StartBroadcasterContext broadcasts dirty signals until ctx is canceled.
func (l *LAN) StartBroadcasterContext(ctx context.Context, usageChan <-chan model.TokenUsage) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-usageChan:
			if !ok {
				return
			}
			l.AnnounceDirty()
		}
	}
}
