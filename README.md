# Infrasible Monitoring Agent

Linux monitoring daemon in Go: systemd non-root service, host metrics (Family modules + Registry), local SQLite, outbound-only Live (WSS/mTLS) and Persistence (HTTPS/mTLS). No inbound management port.

**Current version:** [`VERSION.md`](VERSION.md) (single line `X.Y.Z` — the only place the number is stored)  
**Ship verdict:** [docs/STATUS.md](docs/STATUS.md) · **Doc map:** [docs/DOCUMENTATION.md](docs/DOCUMENTATION.md)

Staging-ready for offline collection and lab Live Hello. **Not** production-certified against central Relay/PKI.

## Docs

| Doc | Purpose |
|---|---|
| [VERSION.md](VERSION.md) | **Semver SSOT** — one line only |
| [Documentation map](docs/DOCUMENTATION.md) | Where each fact lives |
| [Status / verdict](docs/STATUS.md) | What works, what is blocked, next work |
| [Monitoring Relay guide](docs/design/MONITORING_RELAY.md) | Agent Live wire contract for Relay |
| [Relay implementer brief](relay/PROMPT.md) | Full junior build guide (phases under `relay/`) |
| [CI / CD](docs/CI_CD.md) | Verify, version bump, tag-only release |
| [Tests](docs/TEST.md) | Local and CI-equivalent tests |
| [Installation](docs/INSTALLATION.md) | Install package, offline staging |
| [Configuration](docs/CONFIGURATION.md) | JSON config, `enabled_families` |
| [Operations](docs/DAEMON_RUNBOOK.md) | Health, storage, cleanup |
| [Collector families](docs/design/COLLECTOR_FAMILIES.md) | Add metric modules |
| [Example module](docs/design/example_module.md) | Worked Family walkthrough |
| [Protocol & security](docs/design/AGENT_PROTOCOL_AND_SECURITY.md) | Target + executable wire |
| [Hello / heartbeat](docs/design/AGENT_LIVE_HELLO_HEARTBEAT.md) | Live negotiate slice |
| Design targets | [Architecture](docs/design/AGENT_ARCHITECTURE.md), [SQLite](docs/design/AGENT_SQLITE_SCHEMA.md), [Requirements](docs/design/AGENT_PRODUCT_REQUIREMENTS.md), [Acceptance](docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md), [Backlog](docs/design/AGENT_IMPLEMENTATION_BACKLOG.md) |

Do not duplicate the version number outside [`VERSION.md`](VERSION.md). Do not duplicate pass/fail outside [STATUS.md](docs/STATUS.md).

## Implemented (summary)

See [STATUS.md](docs/STATUS.md). In short: 1 Hz Family/Registry collectors with `enabled_families`, SQLite rollups, operator CLI, Live Hello + `type: live`, Linux packages.

## Ship

Requires Go 1.23+. Prefer GitHub Release artifacts.

```sh
bash scripts/ci-cd.sh last
bash scripts/ci-cd.sh version X.Y.Z   # writes VERSION.md + syncs version.go
bash scripts/ci-cd.sh release X.Y.Z
```

See [CI / CD](docs/CI_CD.md).

**License:** MIT
