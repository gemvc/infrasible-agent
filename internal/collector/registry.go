package collector

import (
	"context"
	"fmt"
	"time"
)

// Registry composes enabled Family modules into one Reader. It owns due-selection
// and frame assembly only; each Family owns its own /proc (or future) I/O.
type Registry struct {
	families   []Family
	previousAt time.Time
}

// NewRegistry returns a Reader over the given families in registration order.
func NewRegistry(families ...Family) *Registry {
	return &Registry{families: append([]Family(nil), families...)}
}

// NewLinux returns the default host metric registry (all DefaultFamilyNames).
func NewLinux() *Registry {
	r, err := NewFromNames(DefaultFamilyNames())
	if err != nil {
		panic("collector: default families must always resolve: " + err.Error())
	}
	return r
}

// NewFromNames builds a Registry from catalog names. Unknown names fail closed.
func NewFromNames(names []string) (*Registry, error) {
	cat := familyCatalog()
	families := make([]Family, 0, len(names))
	for _, name := range names {
		fn, ok := cat[name]
		if !ok {
			return nil, fmt.Errorf("unknown family %q", name)
		}
		families = append(families, fn())
	}
	return NewRegistry(families...), nil
}

// DefaultFamilyNames is the ordered default Linux host set when config omits enabled_families.
func DefaultFamilyNames() []string {
	return []string{"cpu", "memory", "network", "disk_io", "load", "filesystem"}
}

// AllFamilyNames is the catalog allowlist accepted by config validation.
// Future GPU/Docker names are added here when their Family modules ship.
func AllFamilyNames() []string {
	return DefaultFamilyNames()
}

// DefaultCadenceProfile returns cadences for the default Linux family set.
func DefaultCadenceProfile() map[string]int64 {
	return NewLinux().CadenceProfile()
}

// familyCatalog maps stable family names to constructors. Add new families here only.
func familyCatalog() map[string]func() Family {
	return map[string]func() Family{
		"cpu":        func() Family { return &cpuFamily{} },
		"memory":     func() Family { return &memoryFamily{} },
		"network":    func() Family { return &networkFamily{} },
		"disk_io":    func() Family { return &diskIOFamily{} },
		"load":       func() Family { return &loadFamily{} },
		"filesystem": func() Family { return &filesystemFamily{} },
	}
}

// Names returns enabled family names in collection order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.families))
	for i, f := range r.families {
		out[i] = f.Name()
	}
	return out
}

// CadenceProfile maps each enabled family to its cadence in seconds.
func (r *Registry) CadenceProfile() map[string]int64 {
	out := make(map[string]int64, len(r.families))
	for _, f := range r.families {
		out[f.Name()] = f.CadenceSeconds()
	}
	return out
}

// Read builds one frame at now. Only due families appear in Metrics.
// Cancelled contexts return an empty metrics map without collecting.
func (r *Registry) Read(ctx context.Context, now time.Time) Frame {
	f := Frame{ObservedAtMS: now.UTC().UnixMilli(), Metrics: map[string]Observation{}}
	var elapsed time.Duration
	if !r.previousAt.IsZero() {
		elapsed = now.Sub(r.previousAt)
		f.ElapsedMS = elapsed.Milliseconds()
	}
	select {
	case <-ctx.Done():
		return f
	default:
	}
	unix := now.Unix()
	for _, fam := range r.families {
		cadence := fam.CadenceSeconds()
		if cadence <= 0 || unix%cadence != 0 {
			continue
		}
		f.Metrics[fam.Name()] = fam.Collect(ctx, now, elapsed)
	}
	r.previousAt = now
	return f
}
