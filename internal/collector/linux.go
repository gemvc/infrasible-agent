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
	"syscall"
	"time"
)

type Observation struct {
	Status       string `json:"status"`
	MeasuredAtMS int64  `json:"measured_at_ms"`
	Unit         string `json:"unit,omitempty"`
	Value        any    `json:"value,omitempty"`
	Error        string `json:"error,omitempty"`
}
type Frame struct {
	ObservedAtMS int64                  `json:"observed_at_ms"`
	ElapsedMS    int64                  `json:"elapsed_ms"`
	Metrics      map[string]Observation `json:"metrics"`
}
type Reader interface {
	Read(context.Context, time.Time) Frame
}
type Linux struct {
	previousCPU  []uint64
	previousNet  map[string][2]uint64
	previousAt   time.Time
	previousDisk map[string][2]uint64
}

func NewLinux() *Linux { return &Linux{} }
func (l *Linux) Read(ctx context.Context, now time.Time) Frame {
	f := Frame{ObservedAtMS: now.UTC().UnixMilli(), Metrics: map[string]Observation{}}
	if !l.previousAt.IsZero() {
		f.ElapsedMS = now.Sub(l.previousAt).Milliseconds()
	}
	due := func(interval int64) bool { return now.Unix()%interval == 0 }
	add := func(name, unit string, value any, err error) {
		o := Observation{Status: "valid", MeasuredAtMS: time.Now().UTC().UnixMilli(), Unit: unit, Value: value}
		if err != nil {
			o.Status = "error"
			o.Error = err.Error()
			o.Value = nil
		}
		f.Metrics[name] = o
	}
	select {
	case <-ctx.Done():
		return f
	default:
	}
	cpu, e := readCPU()
	if e != nil {
		add("cpu", "percent", nil, e)
	} else if l.previousCPU == nil {
		add("cpu", "percent", nil, errors.New("first CPU baseline"))
	} else {
		v, e := usage(l.previousCPU, cpu)
		add("cpu", "percent", v, e)
	}
	if e == nil {
		l.previousCPU = cpu
	}
	m, e := readMemory()
	add("memory", "bytes", m, e)
	net, e := readNet()
	if e != nil {
		add("network", "bytes_per_second", nil, e)
	} else if l.previousNet == nil || f.ElapsedMS <= 0 {
		add("network", "bytes_per_second", nil, errors.New("first network baseline"))
	} else {
		rates := map[string]map[string]float64{}
		for name, c := range net {
			if p, ok := l.previousNet[name]; ok && c[0] >= p[0] && c[1] >= p[1] {
				seconds := float64(f.ElapsedMS) / 1000
				rates[name] = map[string]float64{"rx": float64(c[0]-p[0]) / seconds, "tx": float64(c[1]-p[1]) / seconds}
			}
		}
		add("network", "bytes_per_second", rates, nil)
	}
	if e == nil {
		l.previousNet = net
	}
	disk, e := readDisk()
	if e != nil {
		add("disk_io", "bytes_per_second", nil, e)
	} else if l.previousDisk == nil || f.ElapsedMS <= 0 {
		add("disk_io", "bytes_per_second", nil, errors.New("first disk baseline"))
	} else {
		rates := map[string]map[string]float64{}
		for name, c := range disk {
			if p, ok := l.previousDisk[name]; ok && c[0] >= p[0] && c[1] >= p[1] {
				seconds := float64(f.ElapsedMS) / 1000
				rates[name] = map[string]float64{"read": float64(c[0]-p[0]) / seconds, "write": float64(c[1]-p[1]) / seconds}
			}
		}
		add("disk_io", "bytes_per_second", rates, nil)
	}
	if e == nil {
		l.previousDisk = disk
	}
	if due(5) {
		v, e := readLoad()
		add("load", "load", v, e)
	}
	if due(30) {
		v, e := readFS()
		add("filesystem", "bytes", v, e)
	}
	l.previousAt = now
	return f
}
func fields(path string) ([]string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return strings.Fields(string(b)), nil
}
func ints(s []string) ([]uint64, error) {
	o := make([]uint64, 0, len(s))
	for _, x := range s {
		n, e := strconv.ParseUint(x, 10, 64)
		if e != nil {
			return nil, e
		}
		o = append(o, n)
	}
	return o, nil
}
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
	avail := m["MemAvailable"]
	if avail == 0 {
		avail = m["MemFree"] + m["Buffers"] + m["Cached"]
	}
	swap := uint64(0)
	if m["SwapTotal"] >= m["SwapFree"] {
		swap = m["SwapTotal"] - m["SwapFree"]
	}
	return map[string]uint64{"total": m["MemTotal"], "available": avail, "swap_total": m["SwapTotal"], "swap_used": swap}, nil
}
func readMemory() (map[string]uint64, error) {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return nil, e
	}
	return memoryFrom(string(b))
}
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
func readFS() (map[string]map[string]uint64, error) {
	f, e := os.Open("/proc/mounts")
	if e != nil {
		return nil, e
	}
	defer f.Close()
	out := map[string]map[string]uint64{}
	s := bufio.NewScanner(f)
	skip := map[string]bool{"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true, "cgroup": true, "cgroup2": true, "overlay": true, "squashfs": true}
	for s.Scan() {
		p := strings.Fields(s.Text())
		if len(p) < 3 || skip[p[2]] {
			continue
		}
		mount := strings.ReplaceAll(p[1], "\\040", " ")
		if strings.HasPrefix(mount, "/proc") || strings.HasPrefix(mount, "/sys") || strings.HasPrefix(mount, "/dev") {
			continue
		}
		var st syscall.Statfs_t
		if e := syscall.Statfs(mount, &st); e != nil {
			return nil, fmt.Errorf("statfs %q: %w", mount, e)
		}
		out[mount] = map[string]uint64{"total": st.Blocks * uint64(st.Bsize), "available": st.Bavail * uint64(st.Bsize)}
	}
	return out, s.Err()
}
