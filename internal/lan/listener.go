package lan

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"token-ray/internal/model"
)

// StartListener joins the multicast group and records discovered peers.
// Token data is not trusted from UDP; peers pull records through HTTP sync.
func (l *LAN) StartListener(outChan chan<- model.TokenUsage) {
	l.StartListenerContext(context.Background(), outChan)
}

// StartListenerContext joins the multicast group and records discovered peers
// until ctx is canceled.
func (l *LAN) StartListenerContext(ctx context.Context, outChan chan<- model.TokenUsage) {
	_ = outChan
	var wg sync.WaitGroup

	processPayload := func(buf []byte, n int, srcAddr *net.UDPAddr) {
		var payload model.TrackPayload
		if err := json.Unmarshal(buf[:n], &payload); err != nil {
			return
		}

		if payload.DeviceID == l.DeviceID {
			return
		}

		ipStr := ""
		if srcAddr != nil && srcAddr.IP != nil {
			ipStr = srcAddr.IP.String()
		}

		l.recordPeer(payload.DeviceID, ipStr, payload.HTTPPort, payload.Summary, payload.Type == "dirty")
	}

	readLoop := func(conn *net.UDPConn) {
		defer conn.Close()
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		conn.SetReadBuffer(MaxDatagramSize)
		buf := make([]byte, MaxDatagramSize)
		for {
			n, src, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				time.Sleep(1 * time.Second)
				continue
			}
			processPayload(buf, n, src)
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
		if err == nil {
			if conn, err := net.ListenMulticastUDP("udp", nil, addr); err == nil {
				readLoop(conn)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:9101")
		if err == nil {
			if conn, err := net.ListenUDP("udp", addr); err == nil {
				readLoop(conn)
			}
		}
	}()

	wg.Wait()
}
