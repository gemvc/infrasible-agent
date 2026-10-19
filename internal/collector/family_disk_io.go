package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type diskIOFamily struct {
	previous map[string][2]uint64
}

func (d *diskIOFamily) Name() string          { return "disk_io" }
func (d *diskIOFamily) CadenceSeconds() int64 { return 1 }

func (d *diskIOFamily) Collect(_ context.Context, _ time.Time, elapsed time.Duration) Observation {
	disk, e := readDisk()
	if e != nil {
		return observation("bytes_per_second", nil, e)
	}
	elapsedMS := elapsed.Milliseconds()
	if d.previous == nil || elapsedMS <= 0 {
		d.previous = disk
		return observation("bytes_per_second", nil, errors.New("first disk baseline"))
	}
	rates := map[string]map[string]float64{}
	seconds := float64(elapsedMS) / 1000
	for name, c := range disk {
		if p, ok := d.previous[name]; ok && c[0] >= p[0] && c[1] >= p[1] {
			rates[name] = map[string]float64{"read": float64(c[0]-p[0]) / seconds, "write": float64(c[1]-p[1]) / seconds}
		}
	}
	d.previous = disk
	return observation("bytes_per_second", rates, nil)
}

// readDisk opens /proc/diskstats and converts sector counters to bytes.
func readDisk() (map[string][2]uint64, error) {
	f, e := os.Open("/proc/diskstats")
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return diskFrom(f)
}

// diskFrom rejects malformed nonempty records rather than silently reporting
// a partial disk inventory as a valid measurement.
func diskFrom(r io.Reader) (map[string][2]uint64, error) {
	out := map[string][2]uint64{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		p := strings.Fields(s.Text())
		if len(p) == 0 {
			continue
		}
		if len(p) < 10 {
			return nil, fmt.Errorf("short diskstats record: %q", s.Text())
		}
		// Kernel diskstats sectors are 512-byte units.
		read, e1 := strconv.ParseUint(p[5], 10, 64)
		write, e2 := strconv.ParseUint(p[9], 10, 64)
		if e1 != nil || e2 != nil || read > ^uint64(0)/512 || write > ^uint64(0)/512 {
			return nil, fmt.Errorf("invalid diskstats counters for %q", p[2])
		}
		out[p[2]] = [2]uint64{read * 512, write * 512}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
