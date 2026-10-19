package collector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

type filesystemFamily struct{}

func (f *filesystemFamily) Name() string          { return "filesystem" }
func (f *filesystemFamily) CadenceSeconds() int64 { return 30 }

func (f *filesystemFamily) Collect(_ context.Context, _ time.Time, _ time.Duration) Observation {
	v, e := readFS()
	return observation("bytes", v, e)
}

// readFS opens /proc/mounts and measures capacity via statfs.
func readFS() (map[string]map[string]uint64, error) {
	f, e := os.Open("/proc/mounts")
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return filesystemsFrom(f, syscall.Statfs)
}

// filesystemsFrom inventories non-virtual mounts; any statfs failure fails closed.
func filesystemsFrom(r io.Reader, statfs func(string, *syscall.Statfs_t) error) (map[string]map[string]uint64, error) {
	out := map[string]map[string]uint64{}
	s := bufio.NewScanner(r)
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
		if e := statfs(mount, &st); e != nil {
			return nil, fmt.Errorf("statfs %q: %w", mount, e)
		}
		out[mount] = map[string]uint64{"total": st.Blocks * uint64(st.Bsize), "available": st.Bavail * uint64(st.Bsize)}
	}
	return out, s.Err()
}
