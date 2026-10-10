package collector

import (
	gnet "github.com/shirou/gopsutil/v4/net"
	"net"
	"strings"
	"time"
)

type NetworkSampler struct {
	lastRX, lastTX uint64
	lastTime       time.Time
}

// Physical counters are preferred: ignore loopback and common virtual interfaces to avoid double counting.
func (s *NetworkSampler) Sample() (uint64, uint64, float64, float64, error) {
	counters, err := gnet.IOCounters(true)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	var rx, tx uint64
	for _, c := range counters {
		virtual := false
		for _, prefix := range []string{"lo", "veth", "docker", "br-", "virbr", "tun", "tap", "wg"} {
			if strings.HasPrefix(c.Name, prefix) {
				virtual = true
				break
			}
		}
		if !virtual {
			rx += c.BytesRecv
			tx += c.BytesSent
		}
	}
	now := time.Now()
	var rxRate, txRate float64
	if !s.lastTime.IsZero() {
		elapsed := now.Sub(s.lastTime).Seconds()
		if elapsed > 0 {
			if rx >= s.lastRX {
				rxRate = float64(rx-s.lastRX) / elapsed
			}
			if tx >= s.lastTX {
				txRate = float64(tx-s.lastTX) / elapsed
			}
		}
	}
	s.lastRX, s.lastTX, s.lastTime = rx, tx, now
	return rx, tx, rxRate, txRate, nil
}
func LocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, addr := range addresses {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return ""
}
