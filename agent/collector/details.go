package collector

import (
	"cloudease-lite/shared"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"net"
)

func Details() shared.SystemInfo {
	result := shared.SystemInfo{PrivateIPs: []string{}, PublicIPs: []string{}}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		result.CPUModel = infos[0].ModelName
	}
	if info, err := host.Info(); err == nil {
		result.KernelVersion = info.KernelVersion
		result.Virtualization = info.VirtualizationSystem
		if info.VirtualizationRole != "" {
			result.Virtualization += " (" + info.VirtualizationRole + ")"
		}
	}
	addresses, _ := net.InterfaceAddrs()
	for _, addr := range addresses {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil || ip.IsLoopback() || !ip.IsGlobalUnicast() {
			continue
		}
		if ip.IsPrivate() {
			result.PrivateIPs = append(result.PrivateIPs, ip.String())
		} else {
			result.PublicIPs = append(result.PublicIPs, ip.String())
		}
		if len(result.PrivateIPs)+len(result.PublicIPs) >= 32 {
			break
		}
	}
	return result
}
