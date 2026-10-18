package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLateSampleCannotReopenCommittedQuarter(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "frozen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	begin := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if err = s.Save(ctx, sample(begin, 10)); err != nil {
		t.Fatal(err)
	}
	if err = s.Finalize(ctx, begin.Add(18*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, sample(begin.Add(30*time.Second), 20)); err != nil {
		t.Fatal(err)
	}
	if err = s.Finalize(ctx, begin.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, begin.Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if status.LateRaw != 1 || status.RawUnsaved != 1 || status.Minutes != 0 || status.Pending != 1 {
		t.Fatalf("late sample bypassed immutable quarter: %+v", status)
	}
}
