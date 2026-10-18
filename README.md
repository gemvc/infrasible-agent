# Infrasible Monitoring Agent

Lightweight, open-source Linux host monitoring agent. The agent writes read-only CPU, memory, load, filesystem and network metrics as JSON for SSH-based clients such as Infrasible.

**Status:** released legacy baseline `v0.1.0` (published 2026-10-17). **Current license:** MIT (see LICENSE). **Approved target:** a separate daemon-only v2 design, not yet implemented; Apache-2.0 is conditional on legal review.

## Supported platforms

Linux AMD64 and ARM64; Go 1.23+ for building. No third-party Go dependencies.

## Build and test

```sh
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o infrasible-agent-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o infrasible-agent-linux-arm64 .
```

## Usage on Linux

```sh
./infrasible-agent-linux-amd64 version --json
./infrasible-agent-linux-amd64 snapshot --json
./infrasible-agent-linux-amd64 stream --json
```

`stream` continues until the client terminates the process. Each line contains a JSON snapshot. The agent does not expose a port, run as a service or configure SSH. Installation and SSH session management belong to the client.

See [legacy protocol documentation](docs/PROTOCOL.md) for the **v0.1.0** JSON contract. The **new Agent design is daemon-only** and will remove the public `version/snapshot/stream --json` commands after an explicit Desktop consumer migration; do not replace a currently deployed v1 binary silently.

## Releases

GitHub Actions checks tests and builds both architectures. A tag such as `v0.1.0` matching the version constant initiates a GitHub Release containing Linux executables and simple SHA-256 sidecars. A plain checksum **does not provide publisher authenticity**; cryptographic signing is planned before automatic network updates.

## Approved target architecture (design only)

The owner-approved architecture/DEC-01–11 and **final AD-01–03 refinements (multi-rate collection, truthful freshness/quality and lean single-daemon delivery)**, together with pending evidence gates, live under [docs/design/AGENT_PRODUCT_REQUIREMENTS.md](docs/design/AGENT_PRODUCT_REQUIREMENTS.md), [AGENT_ARCHITECTURE.md](docs/design/AGENT_ARCHITECTURE.md), [AGENT_SQLITE_SCHEMA.md](docs/design/AGENT_SQLITE_SCHEMA.md), [AGENT_PROTOCOL_AND_SECURITY.md](docs/design/AGENT_PROTOCOL_AND_SECURITY.md), and [AGENT_ACCEPTANCE_AND_TEST_MATRIX.md](docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md). [AGENT_IMPLEMENTATION_BACKLOG.md](docs/design/AGENT_IMPLEMENTATION_BACKLOG.md) is the single phased implementation backlog. **G00 is not passed; no daemon implementation, live external contract or automatic deployment is implied by docs.**

### Final daemon-v2 product design (not implemented in v0.1.0)

One small non-root **Go daemon** supervised by systemd, with **one common 1Hz raw frame scheduler**, exactly three SQLite application tables (`server_info`, `raw_samples`, `aggregates`) using WAL/FULL, and independent outgoing **WSS Live** and **HTTPS Persistence** channels secured by mTLS.

| Actual metric family | Approved default collection interval |
|---|---:|
| CPU, Memory/Swap, Network and Disk I/O | 1 second |
| Load Average / runnable tasks | 5 seconds |
| Filesystem free/total capacity | 30 seconds |
| Static host metadata | Startup and detected changes |

**1Hz frame does not mean every metric is remeasured each second.** A metric that is `not_due` is not `missing`. Each due metric carries the real timestamp, unit, source, validity and quality. Real zero, error, missing, counter reset, unsupported and old/stale data are different states. A last-known Live display value must retain its original measured time, age and TTL and **must not** be reinserted as a new raw observation.

UTC one-minute aggregates use the approved **120-second grace** and preserve **per-metric** coverage (e.g. CPU 40/60, Load 8/12, capacity 2/2) and actual measured min/max/peak; immutable fifteen-minute finals preserve that quality for PostgreSQL Persistence. No unobserved inter-check peak is invented. Live Bridge remains stateless; the Agent has no new inbound REST interface, plugin framework, Redis or separate queue database. Independent degraded Health is visible from the first implementation slice.

Development is **Agent-first and vertical-slice driven**: CPU/Memory/Network -> real 1s frames -> SQLite -> 1m/15m -> restart safety/health, then durable Persistence mock and lost-ACK, Live WebSocket, remaining metric cadences, issuer/mTLS/HA and native qualification under the existing P00–P11 gates. This paragraph states the **approved target only**; the currently built and released v0.1.0 is still legacy CLI.

## Relationship with Infrasible

This repository contains **only** the standalone Linux Agent. It does not include the proprietary Infrasible desktop application, local SSH credential vault, inventory, or UI. Changes here do not alter the existing SSH connection implementation.
