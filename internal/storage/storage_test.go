package storage

import (
	"context"
	"encoding/json"
	"github.com/gemvc/infrasible-agent/internal/collector"
	"path/filepath"
	"testing"
	"time"
)

func TestAggregationACKAndProtectedCleanup(t *testing.T) {
	ctx := context.Background()
	db, e := Open(filepath.Join(t.TempDir(), "a.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	base := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 16; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		obs := collector.Observation{Status: "valid", MeasuredAtMS: at.UnixMilli(), Value: map[string]float64{"total_pct": float64(i)}}
		if e = db.Save(ctx, collector.Frame{ObservedAtMS: at.UnixMilli(), Metrics: map[string]collector.Observation{"cpu": obs}}); e != nil {
			t.Fatal(e)
		}
	}
	if e = db.Finalize(ctx, base.Add(18*time.Minute)); e != nil {
		t.Fatal(e)
	}
	st, e := db.Status(ctx, base.Add(61*24*time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if st.Pending != 1 || st.EligibleFinals != 0 || st.Minutes != 1 {
		t.Fatalf("status: %+v", st)
	}
	dry, e := db.Cleanup(ctx, base.Add(61*24*time.Hour), true, 500)
	if e != nil {
		t.Fatal(e)
	}
	if dry.Finals != 0 {
		t.Fatalf("pending marked eligible: %+v", dry)
	}
	p, ok, e := db.OldestPending(ctx)
	if e != nil || !ok {
		t.Fatalf("pending: %v %v", ok, e)
	}
	var final Final
	if e = json.Unmarshal(p.Payload, &final); e != nil {
		t.Fatal(e)
	}
	if final.ResolutionSeconds != 900 || final.ObservedSlots != 15 {
		t.Fatalf("final: %+v", final)
	}
	if e = db.Acknowledge(ctx, p.RecordID, p.Hash, base.Add(20*time.Minute)); e != nil {
		t.Fatal(e)
	}
	result, e := db.Cleanup(ctx, base.Add(61*24*time.Hour), false, 500)
	if e != nil || result.Finals != 1 {
		t.Fatalf("cleanup: %+v %v", result, e)
	}
	st, e = db.Status(ctx, base.Add(61*24*time.Hour))
	if e != nil || st.Pending != 0 || st.RawUnsaved != 0 || st.Finals != 0 {
		t.Fatalf("after: %+v %v", st, e)
	}
}
func TestUnsavedProtected(t *testing.T) {
	ctx := context.Background()
	db, e := Open(filepath.Join(t.TempDir(), "s.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.Save(ctx, collector.Frame{ObservedAtMS: time.Now().UnixMilli(), Metrics: map[string]collector.Observation{}}); e != nil {
		t.Fatal(e)
	}
	r, e := db.Cleanup(ctx, time.Now().Add(100*24*time.Hour), false, 1)
	if e != nil || r.Raw != 0 {
		t.Fatalf("%+v %v", r, e)
	}
	st, e := db.Status(ctx, time.Now())
	if e != nil || st.RawUnsaved != 1 {
		t.Fatalf("%+v %v", st, e)
	}
}

func TestRawAggregationCountsScalarDueOnce(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "scalar.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		at := start.Add(time.Duration(i) * time.Second)
		frame := collector.Frame{
			ObservedAtMS: at.UnixMilli(),
			Metrics: map[string]collector.Observation{
				"scalar": {Status: "valid", MeasuredAtMS: at.UnixMilli(), Value: float64(i + 1)},
				"nested": {Status: "valid", MeasuredAtMS: at.UnixMilli(), Value: map[string]float64{"value": float64(i + 1)}},
			},
		}
		if err := db.Save(ctx, frame); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.db.QueryRowContext(ctx, "SELECT summary_json FROM aggregates WHERE resolution_seconds=60 AND period_start_ms=?", start.UnixMilli()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var metrics map[string]Statistic
	if err := json.Unmarshal([]byte(raw), &metrics); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"scalar", "nested.value"} {
		got := metrics[key]
		if got.Due != 2 || got.Count != 2 || got.Sum != 3 {
			t.Fatalf("%s: got %+v; want due=2 count=2 sum=3", key, got)
		}
	}
}
