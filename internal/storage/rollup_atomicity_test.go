package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// A trigger fails the second SQL step of the minute transaction, after
// aggregate INSERT but before saved=1. The entire transaction must roll back.
func TestMinuteRollupFailureIsAtomic(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "minute-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if err := s.Save(ctx, sample(start, 42)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TEMP TRIGGER fail_minute_saved
BEFORE UPDATE OF saved ON raw_samples
BEGIN SELECT RAISE(ABORT, 'injected minute failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(ctx, start.Add(4*time.Minute)); err == nil {
		t.Fatal("injected minute failure was ignored")
	}
	var minuteRows, savedRows int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aggregates WHERE resolution_seconds=60").Scan(&minuteRows); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM raw_samples WHERE saved=1").Scan(&savedRows); err != nil {
		t.Fatal(err)
	}
	if minuteRows != 0 || savedRows != 0 {
		t.Fatalf("partial minute transaction: aggregates=%d saved=%d", minuteRows, savedRows)
	}
	if _, err := s.db.ExecContext(ctx, "DROP TRIGGER fail_minute_saved"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aggregates WHERE resolution_seconds=60").Scan(&minuteRows); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM raw_samples WHERE saved=1").Scan(&savedRows); err != nil {
		t.Fatal(err)
	}
	if minuteRows != 1 || savedRows != 1 {
		t.Fatalf("minute retry not idempotent: aggregates=%d saved=%d", minuteRows, savedRows)
	}
	if err := s.Integrity(ctx); err != nil {
		t.Fatal(err)
	}
}

// Failing the minute-source DELETE after inserting a quarter aggregate must
// leave all minute rows present and no partial final for later replay.
func TestQuarterRollupFailureIsAtomic(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "quarter-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if err := s.Save(ctx, sample(start.Add(time.Duration(i)*time.Minute), float64(i+1))); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TEMP TRIGGER fail_quarter_delete
BEFORE DELETE ON aggregates WHEN OLD.resolution_seconds=60
BEGIN SELECT RAISE(ABORT, 'injected quarter failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(ctx, start.Add(18*time.Minute)); err == nil {
		t.Fatal("injected quarter failure was ignored")
	}
	var minutes, finals int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aggregates WHERE resolution_seconds=60").Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aggregates WHERE resolution_seconds=900").Scan(&finals); err != nil {
		t.Fatal(err)
	}
	if minutes != 2 || finals != 0 {
		t.Fatalf("partial quarter transaction: minutes=%d finals=%d", minutes, finals)
	}
	if _, err := s.db.ExecContext(ctx, "DROP TRIGGER fail_quarter_delete"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(ctx, start.Add(18*time.Minute)); err != nil {
		t.Fatal(err)
	}
	pending, ok, err := s.OldestPending(ctx)
	if err != nil || !ok {
		t.Fatalf("missing final after retry: %v", err)
	}
	if err := s.Finalize(ctx, start.Add(19*time.Minute)); err != nil {
		t.Fatal(err)
	}
	again, ok, err := s.OldestPending(ctx)
	if err != nil || !ok || again.RecordID != pending.RecordID || again.Hash != pending.Hash {
		t.Fatalf("retry changed immutable final: ok=%v err=%v", ok, err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aggregates WHERE resolution_seconds=60").Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 0 {
		t.Fatalf("minute source rows not retired after durable final: %d", minutes)
	}
	if err := s.Integrity(ctx); err != nil {
		t.Fatal(err)
	}
}
