package collector

import (
	"math"
	"errors"
	"syscall"
	"strings"
	"testing"
)

func TestSwap(t *testing.T) {
	m, e := memoryFrom("MemTotal: 1000 kB\nMemAvailable: 600 kB\nSwapTotal: 100 kB\nSwapFree: 70 kB\n")
	if e != nil || m["swap_used"] != 30720 {
		t.Fatalf("swap: %v %v", m, e)
	}
}
func TestCPUGuestExcluded(t *testing.T) {
	a := []uint64{100, 0, 0, 100, 0, 0, 0, 0, 10, 0}
	b := []uint64{140, 0, 0, 160, 0, 0, 0, 0, 30, 0}
	m, e := usage(a, b)
	if e != nil || math.Abs(m["total_pct"]-40) > 0.01 {
		t.Fatalf("CPU: %v %v", m, e)
	}
}

func TestDiskstatsMalformedInputFailsClosed(t *testing.T) {
	tests := []string{
		"8 0 sda 1 2 3",
		"8 0 sda 1 2 nope 4 5 6 7 8 9 10",
		"8 0 sda 1 2 36028797018963968 4 5 6 7 8 9 10",
	}
	for _, input := range tests {
		if _, err := diskFrom(strings.NewReader(input)); err == nil {
			t.Errorf("invalid diskstats accepted: %q", input)
		}
	}
}

func TestDiskstatsValidSectorsConvertedToBytes(t *testing.T) {
	got, err := diskFrom(strings.NewReader("8 0 sda 1 2 3 4 5 6 7 8 9 10\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got["sda"] != [2]uint64{1536, 3584} {
		t.Fatalf("unexpected disk I/O counters: %v", got)
	}
}

func TestFilesystemStatfsErrorRejectsPartialInventory(t *testing.T) {
	input := "/dev/sda1 /mnt/first ext4 rw 0 0\n/dev/sdb1 /mnt/failure ext4 rw 0 0\n"
	want := errors.New("statfs denied")
	seen := 0
	got, err := filesystemsFrom(strings.NewReader(input), func(path string, st *syscall.Statfs_t) error {
		seen++
		if path == "/mnt/failure" {
			return want
		}
		st.Blocks = 100
		st.Bavail = 50
		st.Bsize = 4096
		return nil
	})
	if !errors.Is(err, want) || got != nil || seen != 2 {
		t.Fatalf("partial filesystem data accepted: got=%v err=%v probes=%d", got, err, seen)
	}
}
