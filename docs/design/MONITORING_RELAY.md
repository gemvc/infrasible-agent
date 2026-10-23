# Monitoring Relay — developer guide

**Audience:** engineers building the central **Monitoring Relay** (Live Bridge) that hosts many Infrasible Agents.  
**You do not need this Agent repository** to implement the Relay. This document is the **wire contract**.  
**Full junior implementer brief** (modules, CI/CD, packaging, build order): [`relay/PROMPT.md`](../../relay/PROMPT.md).

**Companion (Persistence is separate):** Agents also POST 15‑minute finals to a **Persistence API**. That is not the Relay. Do not store monitoring history in the Relay.

**Executable Agent wire (current release):** Hello + heartbeats + `type: live` frames. Target `live_sample` schema is future; accept `type: live` after Hello today. Agent semver: [`VERSION.md`](../../VERSION.md). Ship verdict: [STATUS.md](../STATUS.md).

---

## Readiness and implementation status

Do not maintain a second ship/pass/fail matrix here. Agent readiness and outstanding acceptance are owned by [Agent STATUS](../STATUS.md); the independently implemented Relay owns its [Relay STATUS](https://github.com/gemvc/infrasible-monitoring-server/blob/main/docs/STATUS.md). This document is the executable **wire contract**, not a deployment certification.

The 2026-10-19 Agent v0.3.8 mTLS/mock-Bridge Hello/heartbeat test remains a historical lab fact, not the latest central-integration verdict. An operator reported the integrated Agent → Relay → Desktop Live path working on 2026-10-23; no production PKI or Persistence acceptance follows from that observation.

---

## 1. What you are building

```text
┌─────────────┐  outbound WSS/mTLS   ┌──────────────────┐  auth fan-out   ┌──────────┐
│ Linux Agent │ ──────────────────► │ Monitoring Relay │ ───────────────► │ Desktop  │
│ (many hosts)│   live samples only │ (this service)   │  subscribers    │ viewers  │
└─────────────┘                     └──────────────────┘                  └──────────┘
        │
        │ outbound HTTPS/mTLS (NOT Relay)
        ▼
┌─────────────────┐
│ Persistence API │  ← 900s finals only; durable store
└─────────────────┘
```

**Relay responsibilities**

1. Terminate **many** concurrent Agent Live WebSocket sessions (one active Live session per Agent).
2. Authenticate TLS (mTLS) and **authorize** Agent ID + Live channel (registrar ACL).
3. Negotiate Hello; then accept fresh `type: live` frames and application heartbeats.
4. Forward **current** samples to authorized Desktop subscribers.
5. On Agent disconnect: mark stream stale; **never** replay history from the Relay.

**Relay must not**

- Persist monitoring history / act as time-series DB.
- Accept Persistence finals (wrong channel).
- Treat “valid client certificate” alone as permission to ingest.
- Require Agents to open inbound ports (Agents dial out only).

---

## 2. Trust between Agent and Relay

Trust is **layered**. All layers must pass; any failure → close the session.

### Layer A — Transport (TLS 1.3 + mTLS)

| Side | Checks |
|---|---|
| Agent | Trusts Relay via `ca_file`; verifies `server_name` (SNI/hostname); presents `client_cert_file` + owner-only `client_key_file` (mode `0600`, owned by service user). |
| Relay | Presents server cert for the Live hostname; **requires and verifies** client cert against the issuer/CA used for Agents; reject unknown/expired/revoked client certs. |

Plaintext, wrong hostname, untrusted CA, or missing client cert → no session.

### Layer B — Registration ACL (control plane)

A registry (often separate from the Relay process) binds at least:

- `agent_id` (stable opaque ID from the Agent host DB)
- certificate / public-key identity
- allowed host (operational policy)
- allowed channels: `live` and/or `persistence`
- allowed Live endpoint(s)

**Rule:** mTLS success ≠ authorization. After TLS, Relay looks up the client identity and checks “this `agent_id` may use Live on this endpoint.” Unregistered or wrong-channel Agents are denied even with a cryptographically valid cert.

Enrollment, CSR, issuance, rotation, and revocation are **PKI/registrar** work. The Agent does not self-enroll in v0.3.x; operators stage issued files onto the host.

### Layer C — Application Hello (session negotiate)

After the WebSocket is up, the **first** Agent message is `agent_hello`. Relay replies `hello_accepted` only if Layer B allows this Agent on `live`. Hello confirms protocol major, identity echo, channel, and heartbeat interval. It does **not** grant new authority beyond the registry.

### Layer D — Ongoing session integrity

- Application heartbeats both ways (interval from `hello_accepted.heartbeat_seconds`).
- Peer silence ≈ **3×** heartbeat → Agent reconnects; Relay should drop the session and update Desktop “disconnected/stale.”
- Bound message size; reject oversized frames.
- One active Live connection per Agent (new connect may replace the old).

### What does *not* establish trust

- Putting a URL in Agent JSON without issued certs/ACL entry.
- Matching `persistence_domain` strings alone (Persistence concern).
- Desktop SSH access to the Agent host (ops only; not Live trust).

---

## 3. Hosting many Agents

Design for **N Agents × 1 Live WSS each**, plus Desktop subscriber fan-out.

| Concern | Guidance |
|---|---|
| Connection model | Long-lived WSS; Agent originates dial to `wss://…/v2/agents/live` (path is conventional; agree in deploy). |
| Session map | Key by `agent_id` (from Hello, must match cert/ACL binding). |
| Concurrency | Independent goroutines/tasks per session; no global lock on frame path. |
| Backpressure | If Desktop subscribers are slow, drop **oldest live** frames for that Agent; never block Agents into unbounded Relay buffers. Count drops. |
| Scale-out | Sticky or shared session registry if multiple Relay instances; still **one active Live** per Agent globally. |
| Isolation | One bad Agent must not stall others (timeouts, size caps, per-session memory limits). |
| Observability | Per-Agent: connected, last frame time, last heartbeat, drops, auth reject reasons (no secrets in logs). |

---

## 4. Wire protocol you must implement (current executable Agent)

### 4.1 Connect

1. Accept TLS 1.3 WebSocket with client-cert required.
2. Resolve client cert → registered `agent_id` (deny if unknown).
3. Read **first** text JSON message — must be `agent_hello`.
4. Validate and respond `hello_accepted` (or close).
5. Loop: read `type: live` and `type: heartbeat`; optionally write Relay heartbeats so the Agent silence timer does not fire.

### 4.2 `agent_hello` (Agent → Relay)

```json
{
  "type": "agent_hello",
  "protocol_major": 2,
  "protocol_minor": 0,
  "agent_id": "<stable-agent-id>",
  "agent_version": "<semver>",
  "channel": "live",
  "schema_versions": ["2.0"],
  "capabilities": ["live_1hz", "sample_quality", "metric_cadence_v1", "metric_freshness_v1"],
  "sent_at_utc": "<RFC3339 UTC>"
}
```

Reject if: `type` wrong, `protocol_major` ≠ 2, `channel` ≠ `live`, `agent_id` empty or ≠ ACL binding for this cert, Agent not allowed on Live.

### 4.3 `hello_accepted` (Relay → Agent)

```json
{
  "type": "hello_accepted",
  "protocol_major": 2,
  "protocol_minor": 0,
  "schema_version": "2.0",
  "authorized_agent_id": "<same agent_id>",
  "authorized_channel": "live",
  "endpoint_id": "<this-relay-endpoint-id>",
  "heartbeat_seconds": 15,
  "max_message_bytes": 262144
}
```

Rules the **Agent** enforces (fail-closed): `authorized_agent_id` must match local ID; `authorized_channel` must be `live`; `protocol_major` = 2; `heartbeat_seconds` in `(0, 300]`. Prefer **15** unless you jointly change it. Echo the Agent’s ID exactly.

### 4.4 Live sample frame (Agent → Relay) — current executable shape

```json
{
  "type": "live",
  "agent_id": "<stable-agent-id>",
  "frame": {
    "observed_at_ms": 1792324801000,
    "elapsed_ms": 1000,
    "metrics": {
      "cpu": {
        "status": "valid",
        "measured_at_ms": 1792324801000,
        "unit": "percent",
        "value": {"total_pct": 12.5}
      }
    }
  }
}
```

- ~1 Hz when connected; metric set varies by collector due-ness.
- **Do not require** proposed `type: live_sample` yet; plan a later dual-read migration.
- Never treat Live frames as durable history.

### 4.5 Heartbeat (both directions)

```json
{ "type": "heartbeat", "sent_at_utc": "2026-10-18T12:00:00Z" }
```

Agent sends on `heartbeat_seconds`. Relay should also send heartbeats (or other traffic) often enough that the Agent does not see ~3× silence.

---

## 5. Session lifecycle (pseudocode)

```text
on_tls_websocket_accept:
  identity = verify_client_cert()
  acl = registrar.lookup(identity)          # fail → close
  msg = read_first_json()                  # timeout ~10s
  if msg.type != "agent_hello": close
  if not authorize_live(acl, msg.agent_id): close
  write hello_accepted(agent_id, heartbeat_seconds=15)
  bind session[agent_id] = conn            # replace older session if any
  loop:
    msg = read_json()
    switch msg.type:
      live:      validate size; fan_out_to_subscribers(agent_id, msg); touch last_frame
      heartbeat: touch last_peer
      other:     ignore or close (document choice)
    if peer_silent > 3*heartbeat: close
  on_close: mark Desktop stale; clear session[agent_id] if still this conn
```

---

## 6. Desktop subscribers

- Separate authenticated channel (spec owned by Desktop/Relay team).
- Forward **fresh** frames only; on Agent reconnect, start from new samples (no backlog).
- Authorization: which Desktops may see which `agent_id`s.
- UI must show disconnected/stale when the Agent session drops.

---

## 7. Agent configuration you will see in the field

Operators place issued materials on the host and configure (shape only):

```json
{
  "config_version": 1,
  "data_path": "/var/lib/infrasible-agent/agent.db",
  "monitoring_servers": [
    {
      "id": "monitor-primary",
      "live_url": "wss://relay.example.com/v2/agents/live",
      "server_name": "relay.example.com",
      "ca_file": "/etc/infrasible-agent/ca.pem",
      "client_cert_file": "/var/lib/infrasible-agent/client.crt",
      "client_key_file": "/var/lib/infrasible-agent/client.key"
    }
  ]
}
```

Private key must be readable only by the Agent service user (`0600`). Persistence fields are optional and unused by the Relay.

---

## 8. Acceptance tests (Relay side)

| # | Case | Expected |
|---|---|---|
| R1 | Unregistered client cert | No ingest; connection denied or closed at Hello |
| R2 | Registered Agent, happy path | Hello first → accept → `live` + heartbeats |
| R3 | Wrong `authorized_agent_id` in accept | Agent disconnects/reconnects (do not do this) |
| R4 | First message not Hello | Close; no frames processed |
| R5 | Agent disconnect | Subscribers see stale; no history replay |
| R6 | Slow subscribers | Bounded drop of oldest live; other Agents unaffected |
| R7 | Second connect same `agent_id` | Single active session (document replace policy) |
| R8 | Oversized frame | Reject/close; no huge allocation |
| R9 | Persistence URL traffic | Not accepted on Live listener |

Cross-check Agent mock expectations: `internal/transport/contract_test.go` in the Agent repo (optional; this guide is sufficient).

---

## 9. Out of scope for the Relay (do not implement here)

| Topic | Owner |
|---|---|
| 900s final POST + durable ACK | Persistence API |
| Historical charts | Historical API / Desktop |
| Agent install, SSH, SQLite cleanup | Operator / Desktop via SSH |
| Certificate issuance / rotation | PKI / registrar |
| Proposed `live_sample` migration | Joint Agent + Relay change |

---

## 10. Suggested Relay module layout

You may use any language. Logical packages:

1. **tlsfront** — TLS 1.3, client-cert required, hostname certs  
2. **registrar** — cert → `agent_id` + channel ACL (cache OK, revoke-aware)  
3. **livesession** — Hello, frame loop, heartbeat, silence, session map  
4. **fanout** — Desktop subscriptions, backpressure drops  
5. **observe** — metrics/logs without secrets  

---

## 11. References

- Agent install / offline staging: [INSTALLATION.md](../INSTALLATION.md)  
- Agent config fields: [CONFIGURATION.md](../CONFIGURATION.md)  
- Full target + executable wire notes: [AGENT_PROTOCOL_AND_SECURITY.md](AGENT_PROTOCOL_AND_SECURITY.md) §4–5 and §12  
- Hello slice (Agent-side history): [AGENT_LIVE_HELLO_HEARTBEAT.md](AGENT_LIVE_HELLO_HEARTBEAT.md)  
- Project status: [STATUS.md](../STATUS.md)
