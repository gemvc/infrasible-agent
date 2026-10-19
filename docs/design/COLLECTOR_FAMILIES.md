# Collector families — extension guide

**Audience:** engineers adding host metric families (GPU, Docker, …) to the Agent.

The Agent uses **compile-time Go modules** behind a `Family` contract and a `Registry`. There is **no** dynamic plugin runtime (no `.so`, scripts, or inbound loaders). This matches product AD-03 / FR-18.

## Contracts

| Type | Responsibility |
|---|---|
| `collector.Family` | One metric family: `Name`, `CadenceSeconds`, `Collect` |
| `collector.Registry` | Due-selection + frame assembly; implements `Reader` |
| `config.enabled_families` | Operator allowlist resolved at process start |
| `storage.SetCadenceProfile` | Coverage denominators match the same enabled set |

Wire shape (`Frame` / `Observation` JSON) stays stable. Disabled or not-due families are **omitted** from a frame, not marked missing.

## How to add a family (e.g. `gpu` or `docker`)

1. **Implement** `Family` in a new file such as `internal/collector/family_gpu.go` (SRP: parsers + Collect only).
2. **Register** a constructor in `familyCatalog()` inside `internal/collector/registry.go`.
3. **Export** the name from `AllFamilyNames()` (and optionally `DefaultFamilyNames()` if it should be on by default).
4. **Tests:** pure parser/table tests + registry due/omit + config allowlist acceptance.
5. **Docs:** list the name and cadence in [CONFIGURATION.md](../CONFIGURATION.md).

You do **not** change `agent.Run`, Live/Persistence transport, or rollup math for a new family. Storage coverage follows whatever cadence profile the composition root injects from the Registry.

## Operator enablement

```json
"enabled_families": ["cpu", "memory", "network", "disk_io", "load", "filesystem", "gpu"]
```

Config changes require **daemon restart**. `config validate` reports `restart_required: true`. There is no hot reload of collectors.

## Cadence and quality

- Declare a fixed positive `CadenceSeconds` (engineering defaults today: 1 / 5 / 30).
- Return `status: error` or `unsupported` with structured `error` — never invent a valid zero when the source is unavailable.
- Bound entity cardinality (devices, containers, networks). Fail closed on partial inventories when that is the family policy (same pattern as filesystem).

## Out of scope for this guide

- Implementing GPU or Docker collectors (separate phase).
- Hot-swapping families without restart.
- External plugin loading.
