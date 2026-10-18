package storage

import (
	"context"
	"github.com/gemvc/infrasible-agent/internal/collector"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthNeverClaimsRuntimeConnectivity(t *testing.T) {
	ctx := context.Background()
	s, e := Open(filepath.Join(t.TempDir(), "health.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	begin := time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC)
	f := collector.Frame{ObservedAtMS: begin.UnixMilli(), Metrics: map[string]collector.Observation{}}
	if e = s.Save(ctx, f); e != nil {
		t.Fatal(e)
	}
	fresh, e := s.Health(ctx, begin.Add(5*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if !fresh.CollectorFresh || fresh.LiveConnection != "unknown_from_local_cli" || fresh.PersistenceConnection != "unknown_from_local_cli" {
		t.Fatalf("%+v", fresh)
	}
	stale, e := s.Health(ctx, begin.Add(11*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if stale.CollectorFresh {
		t.Fatal("stale data misreported fresh")
	}
}
