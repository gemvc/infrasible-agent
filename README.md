# Infrasible Monitoring Agent

**Development status: v2 daemon implementation candidate (PR #11).** This branch now contains a Go Linux daemon, pure-Go SQLite, 1Hz observation frames, UTC 1-minute and 15-minute rollups, guarded cleanup, proposed outbound mTLS Live/Persistence clients and systemd packaging. This is **not yet an accepted production release**. The latest published v0.1.0 is a distinct legacy CLI reference. No migration/legacy compatibility is required for the new greenfield service.

**Current license:** MIT. Apache-2.0 remains subject to provenance/legal approval.

## Build and verification

```sh
go mod tidy
go vet ./...
go test -race -count=1 ./...
bash scripts/package-linux.sh
```

Go 1.23+, pure Go driver. CI builds Linux amd64, arm64, armv7 and armv6 artifacts; cross-compilation does **not** prove native CPU/runtime qualification on ARM.

## Run locally on a disposable Linux host

Start from [docs/DAEMON_RUNBOOK.md](docs/DAEMON_RUNBOOK.md). Configure `/etc/infrasible-agent/config.json` using [packaging/config.example.json](packaging/config.example.json) and an authorized non-root service identity.

```sh
infrasible-agent config validate --config=/etc/infrasible-agent/config.json
infrasible-agent run --config=/etc/infrasible-agent/config.json
infrasible-agent health status --config=/etc/infrasible-agent/config.json
infrasible-agent storage status --config=/etc/infrasible-agent/config.json
infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
infrasible-agent storage cleanup --safe --config=/etc/infrasible-agent/config.json
```

Storage cleanup deletes only already aggregated raw, minutes with committed quarters, and final 15-minute rows acknowledged durably and expired 60 days after the period end. Pending finals and unsaved raw remain protected. SQLite deletion does not guarantee immediate filesystem byte reclamation. Keep the database and WAL together.

The `version/snapshot/stream --json` CLI is not part of v2. See [docs/PROTOCOL.md](docs/PROTOCOL.md) for historical v1 behavior. This Agent never opens a public inbound management port. Desktop integration uses operator-authorized existing SSH/Ansible; Live/History UI belong to external services.

## Release and boundaries

GitHub Actions verifies Go tests, race tests, 4 Linux architecture builds and package checks. Release is manual and restricted to main; **no release should be published before external API/PKI signoff, signed-artifact supply chain review, native platform/runtime tests, fault/soak tests and Desktop integration acceptance.** SHA-256 sidecars detect accidental corruption, not publisher authenticity.

## Approved target architecture (design only)

The owner-approved architecture/DEC-01–11 and **final AD-01–03 refinements (multi-rate collection, truthful freshness/quality and lean single-daemon delivery)**, together with pending evidence gates, live under [docs/design/AGENT_PRODUCT_REQUIREMENTS.md](docs/design/AGENT_PRODUCT_REQUIREMENTS.md), [AGENT_ARCHITECTURE.md](docs/design/AGENT_ARCHITECTURE.md), [AGENT_SQLITE_SCHEMA.md](docs/design/AGENT_SQLITE_SCHEMA.md), [AGENT_PROTOCOL_AND_SECURITY.md](docs/design/AGENT_PROTOCOL_AND_SECURITY.md), and [AGENT_ACCEPTANCE_AND_TEST_MATRIX.md](docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md). [AGENT_IMPLEMENTATION_BACKLOG.md](docs/design/AGENT_IMPLEMENTATION_BACKLOG.md) is the single phased implementation backlog. **G00 remains open: code has been started, but no production external contract, sign-off or automatic deployment is implied.**

### Approved daemon-v2 target (implementation partially covered by this branch)

One small non-root **Go daemon** supervised by systemd, with **one common 1Hz raw frame scheduler**, exactly three SQLite application tables (`server_info`, `raw_samples`, `aggregates`) using WAL/FULL, and independent outgoing **WSS Live** and **HTTPS Persistence** channels secured by mTLS.

| Actual metric family | Approved default collection interval |
|---|---:|
| CPU, Memory/Swap, Network and Disk I/O | 1 second |
| Load Average / runnable tasks | 5 seconds |
| Filesystem free/total capacity | 30 seconds |
| Static host metadata | Startup and detected changes |

**1Hz frame does not mean every metric is remeasured each second.** A metric that is `not_due` is not `missing`. Each due metric carries the real timestamp, unit, source, validity and quality. Real zero, error, missing, counter reset, unsupported and old/stale data are different states. A last-known Live display value must retain its original measured time, age and TTL and **must not** be reinserted as a new raw observation.

UTC one-minute aggregates use the approved **120-second grace** and preserve **per-metric** coverage (e.g. CPU 40/60, Load 8/12, capacity 2/2) and actual measured min/max/peak; immutable fifteen-minute finals preserve that quality for PostgreSQL Persistence. No unobserved inter-check peak is invented. Live Bridge remains stateless; the Agent has no new inbound REST interface, plugin framework, Redis or separate queue database. Health/degraded observability is an acceptance requirement; error logs and storage CLI currently cover only part of this requirement.

Remaining development follows **Agent-first vertical slices**: CPU/Memory/Network -> real 1s frames -> SQLite -> 1m/15m -> restart safety/health, then durable Persistence mock and lost-ACK, Live WebSocket, remaining metric cadences, issuer/mTLS/HA and native qualification under the existing P00–P11 gates. This paragraph defines the target, not a claim that all gates have passed.

## Relationship with Infrasible

This repository contains **only** the standalone Linux Agent. It does not include the proprietary Infrasible desktop application, local SSH credential vault, inventory, or UI. Changes here do not alter the existing SSH connection implementation.

### Desktop operator management (local CLI implemented; Desktop integration pending)

The Agent is installed/administered by an authorized human using existing SSH/Ansible tooling, including the Infrasible Monitoring Desktop. The unattended Agent **does not require Desktop to remain open**, and does not expose a new inbound management HTTP service. Operators manage the single versioned `/etc/infrasible-agent/config.json`; validated atomic reload and explicit restart/effective-config status are target features.

Configuration will distinguish **monitored-host alert thresholds** (CPU, RAM and filesystem use, evaluated/shown by Desktop) from **Agent resource limits** (CPU/RAM systemd/cgroup ceilings applied by privileged operator tooling, plus a disk-storage budget that cannot override Pending protection). The Desktop can inspect Agent SQLite/DB-WAL space, counts of safe-to-clean old data and Pending records, and invoke narrow dry-run/safe-cleanup local maintenance over SSH. Unsaved raw and unACKed final records are never deleted by ordinary cleanup. The local status, dry-run and safe-cleanup CLI are implemented on this branch; the Desktop UI, configuration reload, threshold UX, effective cgroup policy and authorization signoff are not shipped. See design docs and MGMT-01–09 acceptance additions.
