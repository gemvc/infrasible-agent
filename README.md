# Infrasible Monitoring Agent

**Version 0.3.8 (daemon v2).** Linux monitoring daemon in Go: systemd non-root service, host metrics, local SQLite, outbound-only Live (WSS/mTLS) and Persistence (HTTPS/mTLS). No inbound management port.

**Verdict:** staging-ready for offline collection and lab Live Hello. **Not** production-certified against central Relay/PKI. Details: [docs/STATUS.md](docs/STATUS.md).

## Docs

| Doc | Purpose |
|---|---|
| [Status / verdict](docs/STATUS.md) | What works, what is blocked, what to do next |
| [Monitoring Relay guide](docs/design/MONITORING_RELAY.md) | **How to build the Relay that hosts many Agents** (trust + wire) — written for Relay developers who do not know this repo |
| [CI / CD](docs/CI_CD.md) | Verify, version bump, tag-only release |
| [Installation](docs/INSTALLATION.md) | Install package, offline staging checks |
| [Configuration](docs/CONFIGURATION.md) | JSON config, Desktop/SSH operator flow |
| [Operations](docs/DAEMON_RUNBOOK.md) | Health, storage, cleanup, troubleshooting |
| [Protocol & security](docs/design/AGENT_PROTOCOL_AND_SECURITY.md) | Target contract + §12 executable wire |
| [Hello / heartbeat](docs/design/AGENT_LIVE_HELLO_HEARTBEAT.md) | Agent Live negotiate slice (implemented) |
| [Architecture](docs/design/AGENT_ARCHITECTURE.md), [SQLite](docs/design/AGENT_SQLITE_SCHEMA.md), [Requirements](docs/design/AGENT_PRODUCT_REQUIREMENTS.md), [Acceptance](docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md), [Backlog](docs/design/AGENT_IMPLEMENTATION_BACKLOG.md) | Approved targets (not all implemented) |

Code wins for *current commands*. Design docs win for *intended contract*. Gaps are release blockers.

## Implemented

- 1 Hz frame loop: CPU/memory/network/disk 1s, load 5s, filesystem 30s  
- SQLite: `server_info`, `raw_samples`, `aggregates`; minute + 900s rollups; pending finals  
- CLI: `run`, `config validate`, `health status`, `storage status`, `storage cleanup`  
- Live: mTLS WSS, `agent_hello` / `hello_accepted`, `type: live` frames, application heartbeats  
- Linux packages (deb/tar; amd64, arm64, armv7, armv6)

## Not production-ready

Central Relay fan-out, PKI enrollment/rotation, Persistence domain HA, full quality wire (`live_sample`), signed release provenance, native soak/SLO. Empty `monitoring_servers` is the supported offline staging mode.

## Ship

Requires Go 1.23+. Prefer GitHub Release artifacts over local builds.

```sh
bash scripts/ci-cd.sh last
bash scripts/ci-cd.sh version X.Y.Z
bash scripts/ci-cd.sh release X.Y.Z
```

See [CI / CD](docs/CI_CD.md).

**License:** MIT
