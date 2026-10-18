package storage

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
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

func TestQualityStatusSurvivesMinuteAndQuarterRollup(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "quality.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		second int
		status string
		value  any
	}{
		{0, "valid", float64(0)},
		{1, "error", nil},
		{2, "counter_reset", nil},
		{3, "missing", nil},
		{4, "unsupported", nil},
		{5, "not_due", nil},
	}
	for _, tc := range cases {
		at := start.Add(time.Duration(tc.second) * time.Second).UnixMilli()
		if err := db.Save(ctx, collector.Frame{
			ObservedAtMS: at,
			Metrics: map[string]collector.Observation{
				"cpu": {Status: tc.status, MeasuredAtMS: at, Value: tc.value},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// First finalize the minute while its quarter is still open.
	if err := db.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var minuteQuality string
	if err := db.db.QueryRowContext(ctx,
		"SELECT quality_json FROM aggregates WHERE resolution_seconds=60 AND period_start_ms=?",
		start.UnixMilli()).Scan(&minuteQuality); err != nil {
		t.Fatal(err)
	}
	var qm QualityCounts
	if err := json.Unmarshal([]byte(minuteQuality), &qm); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if qm["cpu"][tc.status] != 1 {
			t.Fatalf("minute status %s: %#v", tc.status, qm)
		}
	}
	if err := db.Finalize(ctx, start.Add(18*time.Minute)); err != nil {
		t.Fatal(err)
	}
	p, ok, err := db.OldestPending(ctx)
	if err != nil || !ok {
		t.Fatalf("missing final: %v", err)
	}
	var final Final
	if err := json.Unmarshal(p.Payload, &final); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if final.Quality["cpu"][tc.status] != 1 {
			t.Fatalf("final status %s: %#v", tc.status, final.Quality)
		}
	}
	if final.Summary["cpu"].Count != 1 || final.Summary["cpu"].Sum != 0 {
		t.Fatalf("valid zero lost or invalid measurement counted: %#v", final.Summary["cpu"])
	}
	if final.Summary["cpu"].Due != 5 || final.Summary["cpu"].Failed != 4 {
		t.Fatalf("not_due counted as a failure: %#v", final.Summary["cpu"])
	}
}

func TestCadenceCoverageAndRestartReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restarted.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		at := start.Add(time.Duration(i) * time.Second).UnixMilli()
		metrics := map[string]collector.Observation{
			"cpu": {Status: "valid", MeasuredAtMS: at, Value: float64(i)},
		}
		if i%5 == 0 {
			metrics["load"] = collector.Observation{Status: "valid", MeasuredAtMS: at, Value: float64(i)}
		}
		if i%30 == 0 {
			metrics["filesystem"] = collector.Observation{Status: "valid", MeasuredAtMS: at, Value: float64(i)}
		}
		if err := store.Save(ctx, collector.Frame{ObservedAtMS: at, Metrics: metrics}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := store.db.QueryRowContext(ctx,
		"SELECT quality_json FROM aggregates WHERE resolution_seconds=60 AND period_start_ms=?",
		start.UnixMilli()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var quality QualityCounts
	if err := json.Unmarshal([]byte(raw), &quality); err != nil {
		t.Fatal(err)
	}
	for family, expected := range map[string]int64{"cpu": 60, "load": 12, "filesystem": 2, "network": 60} {
		if quality[family]["expected_due"] != expected {
			t.Fatalf("%s expected_due=%d want %d", family, quality[family]["expected_due"], expected)
		}
	}
	if quality["cpu"]["unobserved_due"] != 0 || quality["load"]["unobserved_due"] != 0 ||
		quality["filesystem"]["unobserved_due"] != 0 || quality["network"]["unobserved_due"] != 60 {
		t.Fatalf("incorrect cadence coverage after restart: %#v", quality)
	}
	if err := store.Finalize(ctx, start.Add(18*time.Minute)); err != nil {
		t.Fatal(err)
	}
	p, ok, err := store.OldestPending(ctx)
	if err != nil || !ok {
		t.Fatalf("pending after restart: %v", err)
	}
	var final Final
	if err := json.Unmarshal(p.Payload, &final); err != nil {
		t.Fatal(err)
	}
	if final.Quality["load"]["expected_due"] != 180 || final.Quality["load"]["unobserved_due"] != 168 ||
		final.Quality["filesystem"]["expected_due"] != 30 || final.Quality["filesystem"]["unobserved_due"] != 28 {
		t.Fatalf("15-minute cadence coverage incorrect: %#v", final.Quality)
	}
}

func TestInterruptedSQLiteTransactionIsRolledBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.db")
	if os.Getenv("INFRASIBLE_TEST_CRASH_CHILD") == "1" {
		db, err := Open(pathFromCrashEnv())
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec("INSERT INTO raw_samples(sample_id, observed_at_ms, effective_at_ms, elapsed_ms, metrics_json, quality_json) VALUES(?,?,?,?,?,?)",
			"uncommitted-crash-row", int64(1), int64(1), int64(0), "{}", "{}")
		if err != nil {
			t.Fatal(err)
		}
		// os.Exit simulates abrupt process death before COMMIT: neither
		// deferred rollback nor db.Close is executed in the child.
		os.Exit(0)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id := db.AgentID()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptedSQLiteTransactionIsRolledBack$")
	cmd.Env = append(os.Environ(), "INFRASIBLE_TEST_CRASH_CHILD=1", "INFRASIBLE_TEST_CRASH_DB="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child failed: %v: %s", err, output)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.AgentID() != id {
		t.Fatal("agent identity changed after abrupt process death")
	}
	if err := db.Integrity(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM raw_samples WHERE sample_id=?", "uncommitted-crash-row").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("uncommitted sample survived process crash")
	}
}

func pathFromCrashEnv() string {
	return os.Getenv("INFRASIBLE_TEST_CRASH_DB")
}

func TestMissingFramesAndInvalidValuesDoNotFabricateCoverage(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	observations := []struct {
		second int
		status string
		value any
	}{
		{0, "valid", float64(0)},
		{30, "error", nil},
		{59, "valid", float64(90)},
	}
	for _, obs := range observations {
		at := start.Add(time.Duration(obs.second) * time.Second).UnixMilli()
		if err := db.Save(ctx, collector.Frame{
			ObservedAtMS: at,
			Metrics: map[string]collector.Observation{
				"cpu": {Status: obs.status, MeasuredAtMS: at, Value: obs.value},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Finalize(ctx, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var summaryJSON, qualityJSON string
	if err := db.db.QueryRowContext(ctx,
		"SELECT summary_json,quality_json FROM aggregates WHERE resolution_seconds=60 AND period_start_ms=?",
		start.UnixMilli()).Scan(&summaryJSON, &qualityJSON); err != nil {
		t.Fatal(err)
	}
	var stats map[string]Statistic
	if err := json.Unmarshal([]byte(summaryJSON), &stats); err != nil {
		t.Fatal(err)
	}
	var quality QualityCounts
	if err := json.Unmarshal([]byte(qualityJSON), &quality); err != nil {
		t.Fatal(err)
	}
	cpu := stats["cpu"]
	if cpu.Count != 2 || cpu.Due != 3 || cpu.Failed != 1 || cpu.Min != 0 ||
		cpu.Max != 90 || cpu.Sum != 90 || cpu.PeakAtMS != start.Add(59*time.Second).UnixMilli() {
		t.Fatalf("incorrect partial minute: %+v", cpu)
	}
	if quality["cpu"]["expected_due"] != 60 || quality["cpu"]["unobserved_due"] != 57 ||
		quality["cpu"]["error"] != 1 || quality["cpu"]["valid"] != 2 {
		t.Fatalf("missing frames misclassified: %#v", quality["cpu"])
	}
	if err := db.Finalize(ctx, start.Add(18*time.Minute)); err != nil {
		t.Fatal(err)
	}
	pending, ok, err := db.OldestPending(ctx)
	if err != nil || !ok {
		t.Fatalf("final missing: %v", err)
	}
	var final Final
	if err := json.Unmarshal(pending.Payload, &final); err != nil {
		t.Fatal(err)
	}
	if final.ObservedSlots != 3 || final.Quality["cpu"]["expected_due"] != 900 ||
		final.Quality["cpu"]["unobserved_due"] != 897 || final.Summary["cpu"].Count != 2 {
		t.Fatalf("fabricated final coverage: %+v", final)
	}
}
