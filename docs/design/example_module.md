# Example module: adding a collector family

This document walks through adding a **new metric family** end-to-end, using a fictional `example` family (same pattern as future `docker` / `gpu`).

Companion: [COLLECTOR_FAMILIES.md](COLLECTOR_FAMILIES.md) · Operator config: [CONFIGURATION.md](../CONFIGURATION.md)

## Mental model

```text
config.enabled_families  -->  Registry  -->  Frame.metrics["example"]
                                  |
                            Family.Collect()
```

- **Family** = one module (collect only).
- **Registry** = due schedule + frame assembly (do not change for a normal new family).
- **Agent / storage / transport** = unchanged.

## Contract

```go
type Family interface {
    Name() string
    CadenceSeconds() int64
    Collect(ctx context.Context, now time.Time, elapsed time.Duration) Observation
}
```

| Method | Purpose |
|--------|---------|
| `Name()` | Stable key in config + JSON (`"example"`) |
| `CadenceSeconds()` | Run every N seconds of Unix epoch (e.g. 5) |
| `Collect` | Read source; return `valid` or `error` observation |

Not-due families are **omitted** from the frame (not marked missing).

---

## Step 1 — Implement the family

Create `internal/collector/family_example.go`:

```go
package collector

import (
	"context"
	"errors"
	"os"
	"time"
)

// exampleFamily is a template for new collectors (Docker, GPU, …).
type exampleFamily struct {
	// Hold baselines here if you compute rates (see networkFamily).
}

func (e *exampleFamily) Name() string          { return "example" }
func (e *exampleFamily) CadenceSeconds() int64 { return 5 }

func (e *exampleFamily) Collect(ctx context.Context, now time.Time, elapsed time.Duration) Observation {
	select {
	case <-ctx.Done():
		return observation("count", nil, ctx.Err())
	default:
	}

	value, err := readExample()
	return observation("count", value, err)
}

func readExample() (map[string]float64, error) {
	// Replace with real source (Docker API, sysfs, …).
	// Fail closed: return error, never invent a fake valid zero.
	if _, err := os.Stat("/tmp/example-source"); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("example source unavailable")
		}
		return nil, err
	}
	return map[string]float64{"widgets": 42}, nil
}
```

Rules of thumb:

- Prefer pure helpers (`readExample`) that unit tests can call without `/proc` or Docker.
- Use `observation(unit, value, err)` so status / `measured_at_ms` stay consistent.
- Bound cardinality (containers, devices). Do not grow unbounded maps.

---

## Step 2 — Register in the catalog

Edit `internal/collector/registry.go`:

```go
func AllFamilyNames() []string {
	return []string{
		"cpu", "memory", "network", "disk_io", "load", "filesystem",
		"example", // NEW: accepted by config validation
	}
}

// Only add to DefaultFamilyNames() if it should be ON when config omits enabled_families.
// For optional features (Docker), leave them out of DefaultFamilyNames.

func familyCatalog() map[string]func() Family {
	return map[string]func() Family{
		"cpu":        func() Family { return &cpuFamily{} },
		"memory":     func() Family { return &memoryFamily{} },
		"network":    func() Family { return &networkFamily{} },
		"disk_io":    func() Family { return &diskIOFamily{} },
		"load":       func() Family { return &loadFamily{} },
		"filesystem": func() Family { return &filesystemFamily{} },
		"example":    func() Family { return &exampleFamily{} }, // NEW
	}
}
```

---

## Step 3 — Operator config

```json
{
  "config_version": 1,
  "data_path": "/var/lib/infrasible-agent/agent.db",
  "enabled_families": [
    "cpu", "memory", "network", "disk_io", "load", "filesystem",
    "example"
  ],
  "monitoring_servers": []
}
```

Then:

```sh
infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl restart infrasible-agent
```

Validate JSON includes `restart_required: true`. No hot reload.

---

## Step 4 — What appears on the wire

When due (e.g. Unix time divisible by 5), a Live/raw frame may contain:

```json
{
  "observed_at_ms": 1792324805000,
  "elapsed_ms": 1000,
  "metrics": {
    "cpu": {
      "status": "valid",
      "measured_at_ms": 1792324805001,
      "unit": "percent",
      "value": { "total_pct": 12.5 }
    },
    "example": {
      "status": "valid",
      "measured_at_ms": 1792324805002,
      "unit": "count",
      "value": { "widgets": 42 }
    }
  }
}
```

If the source fails:

```json
"example": {
  "status": "error",
  "measured_at_ms": 1792324805002,
  "unit": "count",
  "error": "example source unavailable"
}
```

Storage rollups and Live already accept arbitrary family keys. Cadence coverage uses the same enabled set via `SetCadenceProfile`.

---

## Step 5 — Tests (minimum)

```go
package collector

import (
	"testing"
	"time"
)

func TestExampleInRegistry(t *testing.T) {
	r, err := NewFromNames([]string{"example"})
	if err != nil {
		t.Fatal(err)
	}
	if r.CadenceProfile()["example"] != 5 {
		t.Fatal("cadence")
	}
	f := r.Read(t.Context(), time.Unix(5, 0).UTC())
	if _, ok := f.Metrics["example"]; !ok {
		t.Fatal("example missing when due")
	}
}
```

Also extend config tests so `"example"` is allowed and unknown names still fail. Prefer testing `readExample` with fixtures rather than live Docker/GPU.

---

## Checklist

- [ ] `family_example.go` implements `Family`
- [ ] Entry in `familyCatalog()`
- [ ] Name in `AllFamilyNames()` (and optionally `DefaultFamilyNames()`)
- [ ] Unit tests for parsers + registry
- [ ] Document name + cadence in `CONFIGURATION.md`
- [ ] Operator enables in `enabled_families` + **restart**

## Do not change (for a normal family)

- `agent.Run` loop
- Live / Persistence transport
- SQLite schema / rollup algorithms (coverage follows injected cadence automatically)

## Real features next

Replace `example` with `docker` or `gpu`: same steps; only `Collect` / source access differ. See [COLLECTOR_FAMILIES.md](COLLECTOR_FAMILIES.md) for product constraints (no dynamic plugins, fail-closed quality).
