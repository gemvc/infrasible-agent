package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/gemvc/infrasible-agent/internal/collector"
)

func TestRawQualityStoresActualDueFamilyOutcomes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "quality.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC).UnixMilli()
	frame := collector.Frame{
		ObservedAtMS: at,
		Metrics: map[string]collector.Observation{
			"cpu":     {Status: "valid", MeasuredAtMS: at, Value: float64(0)},
			"network": {Status: "error", MeasuredAtMS: at, Error: "unavailable"},
			"disk_io": {MeasuredAtMS: at},
		},
	}
	if err := s.Save(ctx, frame); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT quality_json FROM raw_samples WHERE observed_at_ms=?", at).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var status map[string]string
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatal(err)
	}
	if len(status) != 3 || status["cpu"] != "valid" || status["network"] != "error" ||
		status["disk_io"] != "missing" {
		t.Fatalf("incorrect durable raw metric quality: %#v", status)
	}
	if _, due := status["load"]; due {
		t.Fatal("fabricated a not-due family in raw quality")
	}
}
