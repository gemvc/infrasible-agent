// Package collector samples Linux host metrics into 1 Hz frames with independent
// family cadences (1s/5s/30s). Families that are not due are omitted, not marked missing.
package collector

import (
	"context"
	"time"
)

// Observation is one due metric family's measured outcome for a frame.
type Observation struct {
	Status       string `json:"status"`
	MeasuredAtMS int64  `json:"measured_at_ms"`
	Unit         string `json:"unit,omitempty"`
	Value        any    `json:"value,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Frame is one nominal 1 Hz collection result keyed by family name.
type Frame struct {
	ObservedAtMS int64                  `json:"observed_at_ms"`
	ElapsedMS    int64                  `json:"elapsed_ms"`
	Metrics      map[string]Observation `json:"metrics"`
}

// Reader abstracts metric collection so runtime tests can inject fakes.
type Reader interface {
	Read(context.Context, time.Time) Frame
}

// Family is one compile-time metric module. New families (GPU, Docker, …) implement
// this contract and register in the catalog; Registry and agent cores stay closed.
type Family interface {
	Name() string
	CadenceSeconds() int64
	Collect(ctx context.Context, now time.Time, elapsed time.Duration) Observation
}

// observation builds a valid or error Observation with a fresh measurement time.
func observation(unit string, value any, err error) Observation {
	o := Observation{Status: "valid", MeasuredAtMS: time.Now().UTC().UnixMilli(), Unit: unit, Value: value}
	if err != nil {
		o.Status = "error"
		o.Error = err.Error()
		o.Value = nil
	}
	return o
}
