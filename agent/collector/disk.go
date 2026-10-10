package collector

import "github.com/shirou/gopsutil/v4/disk"

// Disk measures the configured volume rather than summing pseudo-filesystems.
func Disk(path string) (float64, uint64, uint64, error) {
	d, err := disk.Usage(path)
	if err != nil {
		return 0, 0, 0, err
	}
	return d.UsedPercent, d.Total, d.Used, nil
}
