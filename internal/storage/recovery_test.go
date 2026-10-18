package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gemvc/infrasible-agent/internal/collector"
	"path/filepath"
	"testing"
	"time"
)

func sample(at time.Time, n float64) collector.Frame {
	return collector.Frame{ObservedAtMS: at.UnixMilli(), ElapsedMS: 1000, Metrics: map[string]collector.Observation{
		"cpu": {Status: "valid", MeasuredAtMS: at.UnixMilli(), Unit: "percent", Value: map[string]float64{"total_pct": n}},
	}}
}
func TestRestartReplayAndStableIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	ctx := context.Background()
	begin := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	first, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	agentID := first.AgentID()
	for i := 0; i < 15; i++ {
		if e = first.Save(ctx, sample(begin.Add(time.Duration(i)*time.Minute), float64(i))); e != nil {
			t.Fatal(e)
		}
	}
	if e = first.Finalize(ctx, begin.Add(18*time.Minute)); e != nil {
		t.Fatal(e)
	}
	pending, ok, e := first.OldestPending(ctx)
	if e != nil || !ok {
		t.Fatalf("pending %v %v", ok, e)
	}
	if e = first.Close(); e != nil {
		t.Fatal(e)
	}
	second, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	if second.AgentID() != agentID {
		t.Fatal("identity changed on restart")
	}
	again, ok, e := second.OldestPending(ctx)
	if e != nil || !ok {
		t.Fatalf("missing pending on restart: %v", e)
	}
	if again.RecordID != pending.RecordID || again.Hash != pending.Hash || !bytes.Equal(again.Payload, pending.Payload) {
		t.Fatal("wire replay changed after restart")
	}
	if e = second.Finalize(ctx, begin.Add(19*time.Minute)); e != nil {
		t.Fatal(e)
	}
	st, e := second.Status(ctx, begin.Add(19*time.Minute))
	if e != nil || st.Finals != 1 {
		t.Fatalf("duplicate final: %+v %v", st, e)
	}
	if e = second.Acknowledge(ctx, again.RecordID, "wrong-hash", time.Now()); e == nil {
		t.Fatal("wrong ACK accepted")
	}
	_, ok, e = second.OldestPending(ctx)
	if e != nil || !ok {
		t.Fatal("wrong ACK marked final")
	}
	if e = second.Integrity(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestRetentionAnchoredAtPeriodEnd(t *testing.T) {
	ctx := context.Background()
	db, e := Open(filepath.Join(t.TempDir(), "retention.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	start := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if e = db.Save(ctx, sample(start, 21)); e != nil {
		t.Fatal(e)
	}
	if e = db.Finalize(ctx, start.Add(18*time.Minute)); e != nil {
		t.Fatal(e)
	}
	p, ok, e := db.OldestPending(ctx)
	if e != nil || !ok {
		t.Fatal("pending not finalized")
	}
	if e = db.Acknowledge(ctx, p.RecordID, p.Hash, start.Add(19*time.Minute)); e != nil {
		t.Fatal(e)
	}
	justBefore := start.Add(Retention + 15*time.Minute - time.Millisecond)
	r, e := db.Cleanup(ctx, justBefore, false, 500)
	if e != nil || r.Finals != 0 {
		t.Fatalf("premature cleanup %+v %v", r, e)
	}
	r, e = db.Cleanup(ctx, start.Add(Retention+15*time.Minute), false, 500)
	if e != nil || r.Finals != 1 {
		t.Fatalf("retention expiry not honored %+v %v", r, e)
	}
}
func TestLateRawProtectedButDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	db, e := Open(filepath.Join(t.TempDir(), "late.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	begin := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if e = db.Save(ctx, sample(begin, 1)); e != nil {
		t.Fatal(e)
	}
	if e = db.Finalize(ctx, begin.Add(4*time.Minute)); e != nil {
		t.Fatal(e)
	}
	// Arrival after the minute has frozen is never merged or silently deleted.
	if e = db.Save(ctx, sample(begin.Add(20*time.Second), 2)); e != nil {
		t.Fatal(e)
	}
	if e = db.Save(ctx, sample(begin.Add(1*time.Minute), 3)); e != nil {
		t.Fatal(e)
	}
	if e = db.Finalize(ctx, begin.Add(5*time.Minute)); e != nil {
		t.Fatal(e)
	}
	st, e := db.Status(ctx, begin.Add(5*time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if st.LateRaw != 1 || st.RawUnsaved != 1 || st.Minutes != 2 {
		t.Fatalf("late protected / progress incorrect %+v", st)
	}
}
func TestExactlyThreeApplicationTables(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "schema.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	rows, e := db.db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			t.Fatal(e)
		}
		names = append(names, name)
	}
	want := []string{"aggregates", "raw_samples", "server_info"}
	for i := range want {
		if i >= len(names) || want[i] != names[i] {
			t.Fatalf("tables %v, want %v", names, want)
		}
	}
	if len(names) != 3 {
		t.Fatalf("extra application table: %v", names)
	}
}
func TestNoDuplicateACK(t *testing.T) {
	ctx := context.Background()
	s, e := Open(filepath.Join(t.TempDir(), "ack.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	st := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if e = s.Save(ctx, sample(st, 2)); e != nil {
		t.Fatal(e)
	}
	if e = s.Finalize(ctx, st.Add(18*time.Minute)); e != nil {
		t.Fatal(e)
	}
	p, ok, e := s.OldestPending(ctx)
	if e != nil || !ok {
		t.Fatal("pending missing")
	}
	if e = s.Acknowledge(ctx, p.RecordID, p.Hash, time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.Acknowledge(ctx, p.RecordID, p.Hash, time.Now()); e == nil {
		t.Fatal("duplicate ACK unexpectedly rewrote timestamp")
	}
	var payload Final
	if e = json.Unmarshal(p.Payload, &payload); e != nil {
		t.Fatal(e)
	}
	if payload.RecordID != p.RecordID {
		t.Fatalf("%+v", payload)
	}
	var persisted sql.NullInt64
	if e = s.db.QueryRow("SELECT persisted_at_ms FROM aggregates WHERE record_id=?", p.RecordID).Scan(&persisted); e != nil || !persisted.Valid {
		t.Fatal("durable marker lost", e)
	}
}
