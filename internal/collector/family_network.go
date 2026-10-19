package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type networkFamily struct {
	previous map[string][2]uint64
}

func (n *networkFamily) Name() string          { return "network" }
func (n *networkFamily) CadenceSeconds() int64 { return 1 }

func (n *networkFamily) Collect(_ context.Context, _ time.Time, elapsed time.Duration) Observation {
	net, e := readNet()
	if e != nil {
		return observation("bytes_per_second", nil, e)
	}
	elapsedMS := elapsed.Milliseconds()
	if n.previous == nil || elapsedMS <= 0 {
		n.previous = net
		return observation("bytes_per_second", nil, errors.New("first network baseline"))
	}
	rates := map[string]map[string]float64{}
	seconds := float64(elapsedMS) / 1000
	for name, c := range net {
		if p, ok := n.previous[name]; ok && c[0] >= p[0] && c[1] >= p[1] {
			rates[name] = map[string]float64{"rx": float64(c[0]-p[0]) / seconds, "tx": float64(c[1]-p[1]) / seconds}
		}
	}
	n.previous = net
	return observation("bytes_per_second", rates, nil)
}

// readNet parses per-interface RX/TX byte counters from /proc/net/dev.
func readNet() (map[string][2]uint64, error) {
	f, e := os.Open("/proc/net/dev")
	if e != nil {
		return nil, e
	}
	defer f.Close()
	out := map[string][2]uint64{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		i := strings.LastIndexByte(line, ':')
		if i < 0 {
			continue
		}
		v, e := ints(strings.Fields(line[i+1:]))
		if e != nil || len(v) < 16 {
			return nil, fmt.Errorf("invalid network line for %q", line[:i])
		}
		out[strings.TrimSpace(line[:i])] = [2]uint64{v[0], v[8]}
	}
	return out, s.Err()
}
