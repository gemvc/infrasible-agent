package collector

import (
	"context"
	"errors"
	"math"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestMemoryAvailableZeroIsValid(t *testing.T) {
	input := "MemTotal: 1024 kB\nMemAvailable: 0 kB\nMemFree: 300 kB\nBuffers: 50 kB\nCached: 100 kB\n"
	got, err := memoryFrom(input)
	if err != nil {
		t.Fatal(err)
	}
	if got["available"] != 0 {
		t.Fatalf("valid zero MemAvailable was replaced with fallback: %+v", got)
	}
}

func TestMemoryAvailableFallbackOnlyWhenFieldIsAbsent(t *testing.T) {
	input := "MemTotal: 1024 kB\nMemFree: 300 kB\nBuffers: 50 kB\nCached: 100 kB\n"
	got, err := memoryFrom(input)
	if err != nil {
		t.Fatal(err)
	}
	if got["available"] != 450*1024 {
		t.Fatalf("missing MemAvailable did not use documented fallback: %+v", got)
	}
}

func TestLoadAndFilesystemCadenceOmitWhenNotDue(t *testing.T) {
	l := NewLinux()
	ctx := context.Background()
	// Unix 1 is not divisible by 5 or 30: load and filesystem must be omitted.
	notDue := l.Read(ctx, time.Unix(1, 0).UTC())
	if _, ok := notDue.Metrics["load"]; ok {
		t.Fatal("load present when not due")
	}
	if _, ok := notDue.Metrics["filesystem"]; ok {
		t.Fatal("filesystem present when not due")
	}
	// Unix 30 is due for both 5s load and 30s filesystem.
	due := NewLinux().Read(ctx, time.Unix(30, 0).UTC())
	if _, ok := due.Metrics["load"]; !ok {
		t.Fatal("load missing when due at epoch 30")
	}
	if _, ok := due.Metrics["filesystem"]; !ok {
		t.Fatal("filesystem missing when due at epoch 30")
	}
}

func TestCancelledContextSkipsCollection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := NewLinux().Read(ctx, time.Unix(30, 0).UTC())
	if len(f.Metrics) != 0 {
		t.Fatalf("expected empty metrics on cancelled context: %+v", f.Metrics)
	}
}
