package collector

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

type cpuFamily struct {
	previous []uint64
}

func (c *cpuFamily) Name() string           { return "cpu" }
func (c *cpuFamily) CadenceSeconds() int64  { return 1 }

func (c *cpuFamily) Collect(_ context.Context, _ time.Time, _ time.Duration) Observation {
	cpu, e := readCPU()
	if e != nil {
		return observation("percent", nil, e)
	}
	if c.previous == nil {
		c.previous = cpu
		return observation("percent", nil, errors.New("first CPU baseline"))
	}
	v, e := usage(c.previous, cpu)
	c.previous = cpu
	return observation("percent", v, e)
}

// readCPU reads aggregate CPU tick counters from /proc/stat.
func readCPU() ([]uint64, error) {
	b, e := os.ReadFile("/proc/stat")
	if e != nil {
		return nil, e
	}
	first := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(first) < 9 || first[0] != "cpu" {
		return nil, errors.New("invalid /proc/stat")
	}
	return ints(first[1:])
}

// usage computes CPU percentage deltas between two /proc/stat snapshots.
// Guest ticks are already included in user/nice and must not be double-counted.
func usage(a, b []uint64) (map[string]float64, error) {
	if len(a) < 8 || len(b) < 8 {
		return nil, errors.New("short CPU counters")
	}
	var total uint64
	for i := 0; i < 8; i++ {
		if b[i] < a[i] {
			return nil, errors.New("CPU counters reset")
		}
		total += b[i] - a[i]
	}
	if total == 0 {
		return nil, errors.New("zero CPU interval")
	}
	delta := func(i int) uint64 { return b[i] - a[i] }
	idle := delta(3) + delta(4)
	pct := func(n uint64) float64 { return 100 * float64(n) / float64(total) }
	return map[string]float64{"total_pct": pct(total - idle), "user_pct": pct(delta(0) + delta(1)), "system_pct": pct(delta(2)), "iowait_pct": pct(delta(4))}, nil
}
