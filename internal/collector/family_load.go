package collector

import (
	"context"
	"errors"
	"strconv"
	"time"
)

type loadFamily struct{}

func (l *loadFamily) Name() string          { return "load" }
func (l *loadFamily) CadenceSeconds() int64 { return 5 }

func (l *loadFamily) Collect(_ context.Context, _ time.Time, _ time.Duration) Observation {
	v, e := readLoad()
	return observation("load", v, e)
}

// readLoad parses 1/5/15 minute load averages from /proc/loadavg.
func readLoad() (map[string]float64, error) {
	f, e := fields("/proc/loadavg")
	if e != nil {
		return nil, e
	}
	if len(f) < 3 {
		return nil, errors.New("short loadavg")
	}
	m := map[string]float64{}
	for i, k := range []string{"one", "five", "fifteen"} {
		v, e := strconv.ParseFloat(f[i], 64)
		if e != nil {
			return nil, e
		}
		m[k] = v
	}
	return m, nil
}
