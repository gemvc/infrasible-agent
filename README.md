# Infrasible Monitoring Agent

**Current development version: 0.3.1 (daemon v2).** This repository contains a standalone Linux monitoring daemon written in Go. The daemon runs under systemd as a non-root user, measures Linux host metrics, keeps local SQLite history, and has outbound-only WSS Live and HTTPS Persistence clients. This version is **staging-ready for offline/local collection** (empty `monitoring_servers`), **not production-certified** against central Bridge/PKI. There is no inbound HTTP management port.

## Source of truth

- [CI / CD](docs/CI_CD.md) — local verify, GitHub CI gate, tag-only releases (push to main does not deploy).
- [Installation](docs/INSTALLATION.md) — obtain verified CI artifacts, install systemd service and initialize the host.
- [Configuration & Desktop connection workflow](docs/CONFIGURATION.md) — exact v0.3.1 JSON, existing-SSH setup sequence, Relay registration dependencies and operator controls.
- [Actual v0.3.0/v0.3.1 wire vs proposed Relay protocol](docs/design/AGENT_PROTOCOL_AND_SECURITY.md#12-v030-executable-wire-vs-target-protocol--integration-handoff) — implementer handoff and integration blockers.
- [Live Hello and Heartbeat implementation guide](docs/design/AGENT_LIVE_HELLO_HEARTBEAT.md) — planned next Live WSS slice (negotiate + heartbeat; keep `type: live` frames).
- [Operations](docs/DAEMON_RUNBOOK.md) — health, storage inspection, safe cleanup and troubleshooting.
- [Product requirements](docs/design/AGENT_PRODUCT_REQUIREMENTS.md), [Architecture](docs/design/AGENT_ARCHITECTURE.md), [SQLite contract](docs/design/AGENT_SQLITE_SCHEMA.md), [Network/security contract](docs/design/AGENT_PROTOCOL_AND_SECURITY.md), [Acceptance tests](docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md), and [Backlog](docs/design/AGENT_IMPLEMENTATION_BACKLOG.md) — approved targets; **not all requirements are implemented or accepted**.

The implementation in source code takes precedence for *currently available commands*. The design documents take precedence for *intended contract and acceptance criteria*. Differences are release blockers, not permission to weaken the requirements.

## What is implemented

- Linux systemd daemon with a single 1 Hz frame loop and due families: CPU, memory, network and disk I/O at 1s, load at 5s and filesystem capacity at 30s.
- Three SQLite application tables (`server_info`, `raw_samples`, `aggregates`) in WAL/FULL mode with transactional minute and quarter-hour rollups and replayable pending 15-minute payloads.
- Local CLI: `run`, `config validate`, `health status`, `storage status`, `storage cleanup --dry-run` and `storage cleanup --safe`.
- Initial mTLS-protected outbound WebSocket and HTTP clients, packaging for Linux amd64, arm64, armv7 and armv6 (deb postinst creates user/dirs/config for staging), and an automated Go test/cross-build pipeline.

## Development milestone 0.3.1

This patch release builds on v0.3.0 with per-family raw/minute/15-minute quality and cadence coverage, transactional rollup failure regression tests, read-only operator database safety, a safer mTLS private-key read path, and correct handling of valid zero `MemAvailable`. The protocol major/minor and `config_version` stay unchanged. It is a development milestone only; central interoperability, PKI lifecycle, full acceptance and production readiness remain unverified.

## Not yet production-ready

The central Live Bridge and Persistence API protocol and durable ACK must be jointly accepted; PKI enrollment, renewal and revocation, registered-domain failover, complete freshness/coverage reporting, Desktop management UX, signed release provenance, and native durability/resource/soak testing are incomplete. Do not install on production monitoring infrastructure on the strength of CI green checks alone. An empty `monitoring_servers` array is the supported **offline staging** mode.

## Development

Requires Go 1.23 or newer. For a normal host install, **prefer CI or released artifacts**, not local compilation.

```sh
bash scripts/ci-local.sh          # gofmt + tidy + vet + race tests (run before push)
bash scripts/package-linux.sh     # optional local cross-build
```

CI verifies every push/PR but **does not publish releases**. Ship only via tag `vX.Y.Z` or the manual [Agent Release](https://github.com/gemvc/infrasible-agent/actions/workflows/release.yml) workflow — see [CI / CD](docs/CI_CD.md).

**License:** MIT. No licensing change is implied by this release.
