package collector

import "github.com/shirou/gopsutil/v4/mem"

func Memory() (float64, uint64, uint64, error) {
	m, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, 0, err
	}
	return m.UsedPercent, m.Total, m.Used, nil
}
