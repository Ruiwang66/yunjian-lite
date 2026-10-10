// Package collector gathers host metrics through gopsutil for Linux and Windows.
package collector

import (
	"cloudease-lite/shared"
	"fmt"
	"github.com/shirou/gopsutil/v4/host"
)

type Collector struct {
	network  NetworkSampler
	DiskPath string
}

func (c *Collector) Collect() (shared.Report, error) {
	var r shared.Report
	info, err := host.Info()
	if err != nil {
		return r, fmt.Errorf("system: %w", err)
	}
	r.Name, r.OS, r.Platform, r.PlatformVersion, r.Arch, r.Uptime = info.Hostname, info.OS, info.Platform, info.PlatformVersion, info.KernelArch, info.Uptime
	r.CPU, r.CPUCores, err = CPU()
	if err != nil {
		return r, fmt.Errorf("cpu: %w", err)
	}
	r.Memory, r.MemoryTotal, r.MemoryUsed, err = Memory()
	if err != nil {
		return r, fmt.Errorf("memory: %w", err)
	}
	r.Disk, r.DiskTotal, r.DiskUsed, err = Disk(c.DiskPath)
	if err != nil {
		return r, fmt.Errorf("disk: %w", err)
	}
	r.NetworkRX, r.NetworkTX, r.NetworkRXRate, r.NetworkTXRate, err = c.network.Sample()
	if err != nil {
		return r, fmt.Errorf("network: %w", err)
	}
	return r, nil
}
