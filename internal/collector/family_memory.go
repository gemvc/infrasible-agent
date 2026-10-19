package collector

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type memoryFamily struct{}

func (m *memoryFamily) Name() string          { return "memory" }
func (m *memoryFamily) CadenceSeconds() int64 { return 1 }

func (m *memoryFamily) Collect(_ context.Context, _ time.Time, _ time.Duration) Observation {
	v, e := readMemory()
	return observation("bytes", v, e)
}

// memoryFrom parses /proc/meminfo text into total/available/swap byte counters.
// A present MemAvailable of zero is valid; fallback applies only when the field is absent.
func memoryFrom(s string) (map[string]uint64, error) {
	m := map[string]uint64{}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, e := strconv.ParseUint(f[1], 10, 64)
		if e == nil {
			m[strings.TrimSuffix(f[0], ":")] = v * 1024
		}
	}
	if m["MemTotal"] == 0 {
		return nil, errors.New("missing MemTotal")
	}
	avail, hasAvailable := m["MemAvailable"]
	if !hasAvailable {
		avail = m["MemFree"] + m["Buffers"] + m["Cached"]
	}
	swap := uint64(0)
	if m["SwapTotal"] >= m["SwapFree"] {
		swap = m["SwapTotal"] - m["SwapFree"]
	}
	return map[string]uint64{"total": m["MemTotal"], "available": avail, "swap_total": m["SwapTotal"], "swap_used": swap}, nil
}

// readMemory loads and parses /proc/meminfo.
func readMemory() (map[string]uint64, error) {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return nil, e
	}
	return memoryFrom(string(b))
}
