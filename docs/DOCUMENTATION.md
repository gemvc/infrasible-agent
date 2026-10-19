# Documentation map — single sources of truth

Do **not** copy current version numbers or ship verdicts into multiple docs. A release bump must not require editing a pile of markdown files.

## Canonical sources

| Fact | Single source of truth | How to read |
|---|---|---|
| **Agent semver** (packages, tags, Hello `agent_version`) | [`VERSION.md`](../VERSION.md) — **one line**, `X.Y.Z` only | `bash scripts/ci-cd.sh last` |
| **Ship verdict** (what works / blocked / next) | [`STATUS.md`](STATUS.md) | Open that file only |
| **Operator config fields** | [`CONFIGURATION.md`](CONFIGURATION.md) | — |
| **Install / package steps** | [`INSTALLATION.md`](INSTALLATION.md) | Use Release asset names; placeholders, not hardcoded `X.Y.Z` |
| **Ops / troubleshooting** | [`DAEMON_RUNBOOK.md`](DAEMON_RUNBOOK.md) | — |
| **CI / release commands** | [`CI_CD.md`](CI_CD.md) + [`scripts/ci-cd.sh`](../scripts/ci-cd.sh) | `version` writes `VERSION.md` and syncs `version.go` |
| **Tests** | [`TEST.md`](TEST.md) | — |
| **Add a metric Family** | [`design/COLLECTOR_FAMILIES.md`](design/COLLECTOR_FAMILIES.md) + [`design/example_module.md`](design/example_module.md) | — |
| **Executable Live wire for Relay builders** | [`design/MONITORING_RELAY.md`](design/MONITORING_RELAY.md) | Protocol shape, not a version stamp |
| **Target design** (may exceed shipped code) | `docs/design/AGENT_*.md` | Always defer “what ships today” to `STATUS.md` |

`version.go` holds a **synced copy** for the Go binary. It is not independently edited. Tests fail if it drifts from `VERSION.md`.

## Rules

1. **Never** put the current ship version (`X.Y.Z`) in README body copy, INSTALLATION, architecture, protocol, or runbook. Point to [`VERSION.md`](../VERSION.md) and/or [`STATUS.md`](STATUS.md).
2. Historical versions are allowed when they are **immutable facts** (e.g. lab proof used published v0.3.8). Those lines must not claim “current”.
3. Design docs describe **targets**. If text might be read as “already shipped”, point to `STATUS.md`.
4. [`README.md`](../README.md) is an index — it may **link** to `VERSION.md`, not duplicate the number.

## Version bump checklist

```sh
bash scripts/ci-cd.sh version X.Y.Z   # writes VERSION.md + syncs version.go
# Update STATUS.md only if the ship *verdict* or capability set changed.
bash scripts/ci-cd.sh release X.Y.Z
```
