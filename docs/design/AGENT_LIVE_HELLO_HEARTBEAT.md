# Live Hello and Heartbeat

**Status:** implemented. Lab-proven with published **v0.3.8** (2026-10-19). Current ship / verdict: [STATUS.md](../STATUS.md). Semver: [`VERSION.md`](../../VERSION.md).  
**Scope:** Agent outbound Live WSS (`internal/transport`).  
**Relay implementers:** use [MONITORING_RELAY.md](MONITORING_RELAY.md) — you do not need this file.

## What the Agent does

After mTLS dial to `live_url`:

1. Send `agent_hello`  
2. Require `hello_accepted` (fail closed)  
3. Stream `type: live` frames (not yet `live_sample`)  
4. Send heartbeats every `heartbeat_seconds` (default 15; Agent accepts 1–300)  
5. Reconnect if peer silent ≈ 3× heartbeat  

Client key must be a regular file, mode `0600`, owned by the service user.

## Wire (executable)

Agent → Relay:

```json
{
  "type": "agent_hello",
  "protocol_major": 2,
  "protocol_minor": 0,
  "agent_id": "<id>",
  "agent_version": "<semver>",
  "channel": "live",
  "schema_versions": ["2.0"],
  "capabilities": ["live_1hz", "sample_quality", "metric_cadence_v1", "metric_freshness_v1"],
  "sent_at_utc": "<RFC3339>"
}
```

Relay → Agent:

```json
{
  "type": "hello_accepted",
  "protocol_major": 2,
  "protocol_minor": 0,
  "schema_version": "2.0",
  "authorized_agent_id": "<same id>",
  "authorized_channel": "live",
  "endpoint_id": "<relay-id>",
  "heartbeat_seconds": 15,
  "max_message_bytes": 262144
}
```

Then Agent sends `{ "type": "live", "agent_id": "…", "frame": { … } }` and `{ "type": "heartbeat", "sent_at_utc": "…" }`.

Hello does not replace mTLS or registrar ACL. Full trust model: [MONITORING_RELAY.md](MONITORING_RELAY.md) §2. Staging install: [INSTALLATION.md](../INSTALLATION.md). Tests: `internal/transport/contract_test.go`.

## Ordering

```text
[done] offline staging agent
[done] Live Hello + heartbeat + lab Bridge exercise
[now]  production Relay + PKI (external) — MONITORING_RELAY.md
  → optional live_sample migration
  → Persistence failover / enrollment automation
```
