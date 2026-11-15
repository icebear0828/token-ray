package lan

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"token-ray/internal/model"
)

// discoveryHosts merges subnet-sweep targets with explicit unicast targets
// (extra peer hosts + Tailscale auto-discovery), deduplicating the union.
func (l *LAN) discoveryHosts(ctx context.Context) []string {
	subnet := localHTTPDiscoveryHosts()
	static, tailscaleOn := l.extraHostsSnapshot()

	var tailscale []string
	if tailscaleOn {
		tailscale = TailscalePeerHosts(ctx)
	}

	if len(static) == 0 && len(tailscale) == 0 {
		return subnet
	}

	seen := make(map[string]bool, len(subnet)+len(static)+len(tailscale))
	out := make([]string, 0, len(subnet)+len(static)+len(tailscale))
	for _, h := range subnet {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, h := range static {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, h := range tailscale {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

func localHTTPDiscoveryHosts() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	hosts := make([]string, 0)
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrsList, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrsList {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			for _, host := range probeHostsForIPv4(ipnet.IP, ipnet.Mask) {
				if seen[host] {
					continue
				}
				seen[host] = true
				hosts = append(hosts, host)
			}
		}
	}
	sort.Strings(hosts)
	return hosts
}

func probeHostsForIPv4(ip net.IP, mask net.IPMask) []string {
	v4 := ip.To4()
	if v4 == nil {
		return nil
	}
	ones, bits := mask.Size()
	if bits != 32 {
		return nil
	}
	if ones < 24 {
		mask = net.CIDRMask(24, 32)
	}

	ipInt := binary.BigEndian.Uint32(v4)
	maskInt := binary.BigEndian.Uint32(mask)
	network := ipInt & maskInt
	broadcast := network | ^maskInt
	if broadcast <= network+1 {
		return nil
	}

	hosts := make([]string, 0, broadcast-network-1)
	for candidate := network + 1; candidate < broadcast; candidate++ {
		if candidate == ipInt {
			continue
		}
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], candidate)
		hosts = append(hosts, net.IP(buf[:]).String())
	}
	return hosts
}

// StartHTTPDiscovery periodically probes local subnet HTTP endpoints for peers.
func (l *LAN) StartHTTPDiscovery(ctx context.Context) {
	l.ScanHTTPPeers(ctx, nil, nil)

	ticker := time.NewTicker(HTTPDiscoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.ScanHTTPPeers(ctx, nil, nil)
		}
	}
}

// ScanHTTPPeers actively discovers LAN nodes by probing their self endpoint.
//
// When hosts is empty, the scan covers:
//   - hosts on the same IPv4 subnet as each up interface (subnet sweep)
//   - user-configured ExtraPeerHosts (Tailscale, VPN, cross-subnet, ...)
//   - online Tailscale peers when SetTailscaleDiscovery(true) is in effect
//
// Explicitly passing hosts skips all auto-discovery and probes only what's given.
func (l *LAN) ScanHTTPPeers(ctx context.Context, hosts []string, ports []int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(hosts) == 0 {
		hosts = l.discoveryHosts(ctx)
	}
	if len(ports) == 0 {
		ports = l.httpDiscoveryPorts()
	} else {
		ports = normalizeHTTPDiscoveryPorts(ports)
	}
	if len(hosts) == 0 || len(ports) == 0 {
		return
	}

	type target struct {
		host string
		port int
	}
	totalTargets := len(hosts) * len(ports)
	workers := httpDiscoveryConcurrency
	if totalTargets < workers {
		workers = totalTargets
	}

	client := &http.Client{Timeout: HTTPDiscoveryTimeout}
	jobs := make(chan target)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				l.probeHTTPPeer(ctx, client, job.host, job.port)
			}
		}()
	}

sendJobs:
	for _, host := range hosts {
		for _, port := range ports {
			select {
			case <-ctx.Done():
				break sendJobs
			case jobs <- target{host: host, port: port}:
			}
		}
	}
	close(jobs)
	wg.Wait()
}

func (l *LAN) probeHTTPPeer(ctx context.Context, client *http.Client, host string, port int) {
	if port <= 0 || port > 65535 {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, HTTPDiscoveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+net.JoinHostPort(host, strconv.Itoa(port))+"/api/lan/self", nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var self model.LANSelfResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxDatagramSize)).Decode(&self); err != nil {
		return
	}
	if self.DeviceID == "" || self.DeviceID == l.DeviceID {
		return
	}
	httpPort := self.HTTPPort
	if httpPort < 0 || httpPort > 65535 {
		httpPort = 0
	}
	l.recordPeer(self.DeviceID, host, httpPort, self.Summary, true)
}
