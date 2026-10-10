package collector

import (
	"github.com/shirou/gopsutil/v4/cpu"
	"time"
)

func CPU() (float64, int, error) {
	values, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, 0, err
	}
	cores, err := cpu.Counts(true)
	if err != nil {
		return 0, 0, err
	}
	if len(values) == 0 {
		return 0, cores, nil
	}
	return values[0], cores, nil
}
