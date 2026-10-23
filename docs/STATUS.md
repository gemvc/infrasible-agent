# Project status — Infrasible Agent

**Version:** [`VERSION.md`](../VERSION.md) only (`bash scripts/ci-cd.sh last`)  
**Protocol:** daemon major 2 (`protocolMajor` / `protocolMinor` in `version.go` — not the product semver)  
**Updated:** 2026-10-19 (edit this file when the *verdict* or capability set changes — not on every semver bump)

This file is the **only** documentation source of truth for what is staging-ready vs blocked. Other docs must link here instead of restating version or pass/fail tables. Doc map: [DOCUMENTATION.md](DOCUMENTATION.md).

## Final verdict

| Track | Status |
|---|---|
| Offline staging (empty `monitoring_servers`) | **PASS** — collect, SQLite, health/storage CLI, Linux packages |
| Live against lab/mock Bridge (mTLS + Hello + `type: live` + heartbeat) | **PASS** — lab-proven (published v0.3.8, 2026-10-19); wire unchanged for later releases |
| Collector Family registry + `enabled_families` | **PASS** — compile-time modules; config toggle; restart required |
| Central Relay implementation + Desktop fan-out | **IMPLEMENTED / LAB PASS** — [Relay repository](https://github.com/gemvc/infrasible-monitoring-server) reports Go implementation, Agent mTLS lab path and Desktop subscriber endpoint; Desktop operator reported successful Agent → Relay → Desktop Live on 2026-10-23. This is not production PKI/registrar certification. |
| Persistence durable ACK / domain failover | **NOT READY** — external Persistence contract |
| Full acceptance (G00 / P00–P11) | **BLOCKED** on central interfaces |

**Bottom line:** Local collection and lab Live are staging-ready, and the separately maintained Relay now implements Live fan-out; an operator reported successful end-to-end Desktop Live on 2026-10-23. Central Production certification remains blocked by PKI/registrar lifecycle, persistence durable ACK, and full acceptance/security/recovery evidence. See [Relay STATUS](https://github.com/gemvc/infrasible-monitoring-server/blob/main/docs/STATUS.md) for the Relay's own verdict. 

## What to read

| If you… | Read |
|---|---|
| Need the doc map / SSOT rules | [DOCUMENTATION.md](DOCUMENTATION.md) |
| Operate / install the Agent | [INSTALLATION.md](INSTALLATION.md), [CONFIGURATION.md](CONFIGURATION.md), [DAEMON_RUNBOOK.md](DAEMON_RUNBOOK.md), [CI_CD.md](CI_CD.md), [TEST.md](TEST.md) |
| Add a metric family (Docker, GPU, …) | [design/COLLECTOR_FAMILIES.md](design/COLLECTOR_FAMILIES.md), [design/example_module.md](design/example_module.md) |
| Build the Monitoring Relay | [design/MONITORING_RELAY.md](design/MONITORING_RELAY.md) (wire) + [`relay/PROMPT.md`](../relay/PROMPT.md) (full implementer brief) |
| Need wire / security detail | [design/AGENT_PROTOCOL_AND_SECURITY.md](design/AGENT_PROTOCOL_AND_SECURITY.md) §12 (executable) vs §4–6 (target) |
| Need product/architecture targets | [design/AGENT_PRODUCT_REQUIREMENTS.md](design/AGENT_PRODUCT_REQUIREMENTS.md), [design/AGENT_ARCHITECTURE.md](design/AGENT_ARCHITECTURE.md) |

## Next work (priority)

1. **Qualification, not implementation:** validate the already-implemented Relay + Desktop path against production trust/ACL, restart/reconnect, subscriber backpressure and stale semantics. Relay owner: [Relay STATUS](https://github.com/gemvc/infrasible-monitoring-server/blob/main/docs/STATUS.md).  
2. PKI/registrar: enroll, bind cert ↔ `agent_id` ↔ Live ACL, rotate/revoke.  
3. Persistence API: durable ACK fixtures; then optional Agent failover.  
4. Optional later: real `docker` / `gpu` Family modules (extension path already documented).  
5. Optional later: migrate Live frames from `type: live` → `live_sample`.
