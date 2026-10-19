package agent

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gemvc/infrasible-agent/internal/collector"
	"github.com/gemvc/infrasible-agent/internal/config"
	"github.com/gemvc/infrasible-agent/internal/storage"
)

// stubReader returns a fixed frame for every Read call.
type stubReader struct {
	frame collector.Frame
	n     atomic.Int64
}

func (s *stubReader) Read(context.Context, time.Time) collector.Frame {
	s.n.Add(1)
	f := s.frame
	f.ObservedAtMS = time.Now().UTC().UnixMilli()
	return f
}

func TestEnqueueLatestDropsOldestWhenFull(t *testing.T) {
	live := make(chan collector.Frame, 2)
	enqueueLatest(live, collector.Frame{ObservedAtMS: 1})
	enqueueLatest(live, collector.Frame{ObservedAtMS: 2})
	enqueueLatest(live, collector.Frame{ObservedAtMS: 3})
	if len(live) != 2 {
		t.Fatalf("expected capacity retained, got %d", len(live))
	}
	a := <-live
	b := <-live
	if a.ObservedAtMS == 1 {
		t.Fatal("oldest frame was not dropped")
	}
	if a.ObservedAtMS != 2 || b.ObservedAtMS != 3 {
		t.Fatalf("unexpected retained frames: %d %d", a.ObservedAtMS, b.ObservedAtMS)
	}
}

func TestRunExitsOnCancelAndPersistsSamples(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stub := &stubReader{frame: collector.Frame{
		ElapsedMS: 1000,
		Metrics: map[string]collector.Observation{
			"cpu": {Status: "valid", MeasuredAtMS: 1, Value: map[string]float64{"total_pct": 1}},
		},
	}}
	a := &Agent{
		Config:    config.Config{ConfigVersion: 1, MonitoringServers: nil},
		Store:     store,
		Collector: stub,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for stub.n.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}
	if stub.n.Load() < 1 {
		t.Fatal("collector was never invoked")
	}
	st, err := store.Status(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.RawUnsaved+st.RawSaved < 1 {
		t.Fatalf("expected persisted raw samples: %+v", st)
	}
}

func TestRunSurvivesSaveFailure(t *testing.T) {
	// Closed store forces Save errors; Run must log and continue until cancel.
	store, err := storage.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	a := &Agent{
		Config: config.Config{ConfigVersion: 1},
		Store:  store,
		Collector: &stubReader{frame: collector.Frame{
			ElapsedMS: 1000,
			Metrics:   map[string]collector.Observation{"cpu": {Status: "valid", MeasuredAtMS: 1, Value: float64(1)}},
		}},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatal(err)
	}
}
