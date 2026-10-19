# Infrasible Monitoring Agent — Protocol, Registration and Security

> **Note:** Target/acceptance text below. For current shipped behavior and verdict see [STATUS.md](../STATUS.md), [README](../../README.md), and operator docs under `docs/`. Relay builders: [MONITORING_RELAY.md](MONITORING_RELAY.md).

**Status:** target baseline; central Relay/PKI/Persistence still need external sign-off. Agent v0.3.8 implements clients + mocks (Hello Live path lab-proven). **Related:** [MONITORING_RELAY.md](MONITORING_RELAY.md), [STATUS.md](../STATUS.md), [Requirements](AGENT_PRODUCT_REQUIREMENTS.md), [Architecture](AGENT_ARCHITECTURE.md), [SQLite](AGENT_SQLITE_SCHEMA.md), [Acceptance](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md).

## 1. Wire and trust boundaries

| Property | Live | Persistence |
|---|---|---|
| Transport | outbound persistent `wss://` over TLS/mTLS | outbound `https://` requests with keep-alive and TLS/mTLS |
| Destination | stateless **Live Bridge** (per-channel authorized) | **Persistence API** on authorized durable/deduplication domain |
| Payload | **1Hz fresh frame** with actual newly measured 1s/5s/30s observations plus explicit per-metric quality/freshness; no cached metric presented as new | **Only immutable 900s** with metric-specific cadence, coverage and actual measured peaks/quality |
| Destination storage | no monitoring-history DB/read | durable PostgreSQL commit plus idempotent dedup |
| Agent outgoing buffer | small, non-durable, drop oldest (engineering initial 2) | SQLite pending predicate, oldest-first; one active submission initially |
| ACK | optional transport/live receipt; never durable-history proof | authenticated ACK matching agent/period/record/hash/domain and durable commit |
| Failure | reconnect; no historical stream replay | keep local Pending; replay exact unchanged payload |
| Server switching | any eligible authorized Live endpoint | only eligible authorized endpoint in same **verified** shared persistence/dedup domain |
| Inbound | Agent opens all connections; no Agent public listener | same |

All operational endpoints are authenticated and **authorized**. Certificate validity does not automatically grant Agent, host, channel, domain or action permission. Reject plaintext, invalid hostname/SNI, mismatched trust CA, expired/revoked identity, unsupported protocol major, oversized payload and wrong Agent registration.

## 2. Single versioned N-endpoint config

Selected format **JSON**: built-in Go parser, strict schema/unknown critical field validation, deterministic array order, no YAML scalar ambiguity. The array index is the sole priority (index zero highest), and N is not capped at two. Engineering illustrative configuration (URLs, names and files are **not deployed services**):

```json
{
  "config_version": 1,
  "monitoring_servers": [
    {
      "id": "monitor-a",
      "live_url": "wss://a.example.net/v2/agents/live",
      "persistence_url": "https://a.example.net/v2/agents/aggregates",
      "persistence_domain": "primary",
      "server_name": "a.example.net",
      "ca_file": "/etc/infrasible-agent/ca-a.pem",
      "client_cert_file": "/var/lib/infrasible-agent/a.crt",
      "client_key_file": "/var/lib/infrasible-agent/a.key"
    },
    {
      "id": "monitor-b",
      "live_url": "wss://b.example.net/v2/agents/live",
      "persistence_url": "https://b.example.net/v2/agents/aggregates",
      "persistence_domain": "primary",
      "server_name": "b.example.net",
      "ca_file": "/etc/infrasible-agent/ca-b.pem",
      "client_cert_file": "/var/lib/infrasible-agent/b.crt",
      "client_key_file": "/var/lib/infrasible-agent/b.key"
    },
    {
      "id": "monitor-c",
      "live_url": "wss://c.example.net/v2/agents/live",
      "persistence_url": "https://c.example.net/v2/agents/aggregates",
      "persistence_domain": "secondary",
      "server_name": "c.example.net",
      "ca_file": "/etc/infrasible-agent/ca-c.pem",
      "client_cert_file": "/var/lib/infrasible-agent/c.crt",
      "client_key_file": "/var/lib/infrasible-agent/c.key"
    }
  ]
}
```

**Important:** `persistence_domain` is a claimed identifier; the Agent must verify the registered domain and authority through trusted enrollment/Hello/authorization evidence and central owner acceptance tests. Having identical text in two configs **never** proves storage replication or deduplication. Never assume A's ACK proves B's distinct storage contains data.

A candidate can support only one channel when authorized/configured for it; its other channel is ineligible without making the first channel ineligible. Empty N, repeated IDs, invalid schemes, wrong trust roots, invalid key ownership, duplicate/conflicting endpoint identity, unsupported config version and unknown critical fields reject new config atomically.

**Reload protocol:** privileged management writes temp in same directory, validates format and references, fsyncs temp then atomic rename and fsync directory; daemon reads complete new snapshot, validates authorization and switches selection atomically. On failure previous valid config remains in service; removed/revoked endpoint transport must be canceled promptly. No token/password/private-key content inline in config; paths point to root/service-protected files, with symlink/ownership checks.

## 3. Enrollment, identity and PKI

1. Existing Transible SSH workflow or independent operator verifies signed package and installs non-root systemd service plus trusted registration/bootstrap endpoint and one-time short-lived enrollment authority. This does **not** change core Desktop SSH.
2. Agent locally generates CSPRNG `agent_id` and private key; key **never** leaves host as plaintext. Persist stable ID in `server_info`. Prepare CSR including subject/Agent binding according to issuer contract.
3. Agent connects to authenticated issuer via trusted channel using one-time short-lived enrollment token; issuer binds registered Agent ID, approved host, certificate public key, authorized endpoints, allowed Live/Persistence channels and domain. Token must be single-use/revocable, scope-limited, redacted and not left in persisted Agent config.
4. Issued certificate + authority material are stored with strict owner permissions (dedicated service user). Separate per-authority credentials are preferred; common credentials allowed only if explicitly authorized by issuer policy. Agent validates server hostnames/SNI, trust chain, temporal validity and revocation semantics.
5. Rotation initial engineering example: certificates valid ~7 days and renew around two-thirds of lifetime through authorized issuer. Renew before expiry without reusing revoked credentials; expired/unauthorized certificate stops operational connections and enters `AUTH_BLOCKED`. Exact TTL/rotation/revocation SLA is an external PKI contract, not implemented here.
6. If same host is restored, preserve `agent_id` only after central/authorized identity check. Clone must receive a **different** ID and new key/cert and must not be able to use inherited key simultaneously. Hostname and `machine-id` alone cannot detect all clones; issuer must reject/quarantine conflicting registration. Compromised keys require issuer revocation and safe re-enrollment.

**Authorization rule:** a valid mTLS session alone is insufficient. Server must check enrolled `agent_id`, certificate/public-key binding, requested channel operation, allowed destination, persistence-domain authority and policy. Agent validates signed/trusted authorization/domain response; no admission based on untrusted name in a JSON response.

## 4. Hello/negotiation (proposed wire contract)

The new daemon uses its own explicitly versioned network protocol (initial proposed major 2); legacy v1 stdout/SSH is historical reference only and requires no compatibility. Semantic versioning of the process alone is insufficient. Negotiate major, compatible minor and payload schema; reject incompatible major, unknown required capabilities or payload > size cap. The numeric defaults below are engineering starting points for mock tests, not deployed API claims.

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
  "sent_at_utc": "<timestamp>"
}
```

```json
{
  "type": "hello_accepted",
  "protocol_major": 2,
  "protocol_minor": 0,
  "schema_version": "2.0",
  "authorized_agent_id": "<stable-agent-id>",
  "authorized_channel": "live",
  "endpoint_id": "monitor-a",
  "heartbeat_seconds": 15,
  "max_message_bytes": 262144
}
```

Connection context must be authenticated and channel-authorized independently of the envelope. A remote `hello_accepted` cannot extend authority beyond the trusted registrar/ACL. Message size initial cap candidate 256KiB; choose documented tested caps on both sides, include high-cardinality refusal, and fail safely rather than allocate unbounded buffers.

## 5. Live WSS envelope and flow

Illustrative **single new 1Hz Live frame**, with a 5s Load and 30s Filesystem collector not due on this tick (JSON is a proposed v2 schema fixture, not a deployed API):

```json
{
  "type": "live_sample",
  "protocol_major": 2,
  "schema_version": "2.0",
  "agent_id": "<stable-agent-id>",
  "sample_id": "<stable-frame-id>",
  "collected_at_utc": "2026-10-18T12:00:01Z",
  "elapsed_ms": 1008,
  "sampling_profile_version": 1,
  "quality": {
    "clock": "trusted",
    "not_due": ["load", "filesystem_capacity"],
    "errors": []
  },
  "measurements": {
    "cpu": {
      "measured_at_utc": "2026-10-18T12:00:01Z",
      "status": "valid", "unit": "percent", "value": 12.5
    },
    "memory": {
      "measured_at_utc": "2026-10-18T12:00:01Z",
      "status": "valid", "unit": "bytes", "value": "4294967296"
    },
    "disk_io": {
      "measured_at_utc": "2026-10-18T12:00:01Z",
      "status": "unsupported", "value": null
    }
  },
  "last_known_for_display": {
    "filesystem_capacity": {
      "origin": "cache", "new_observation": false,
      "measured_at_utc": "2026-10-18T12:00:00Z",
      "age_ms": 1000, "freshness": "within_ttl",
      "value": {"mount": "/", "available_bytes": "53687091200"}
    }
  }
}
```

**Contract restrictions:** `measurements` may contain **only newly attempted due** readings with original source times and typed statuses. An old display cache, if included at all, lives under distinct `last_known_for_display`, retains its original timestamp, reports age/TTL/freshness and `new_observation=false`; it is **never persisted as a new sample** or treated as a new aggregate event. `not_due` does not count as `missing`. Valid zero differs from null with `error`, `missing`, `counter_reset` or `unsupported`. `stale` is a derived state once age exceeds TTL, not a new measured value. For long-lived views, the UI recomputes age from original timestamp; a healthy WSS heartbeat never refreshes a metric's original observation time.

**Per-metric cadence defaults:** CPU/Network/Memory/Swap/Disk I/O 1s; Load 5s; Filesystem capacity 30s; static host metadata startup/change. On 60-second full healthy operations, per-metric expected opportunities are 60/12/2, distinct from the global 60 frame slots. Emit recognized `sampling_profile_version` and timestamps; negotiating unsupported required cadence/quality schema fails explicitly, never loses the new fields silently.

A single active WSS per Agent/channel, independent of Persistence. Queue initial bounded capacity **2 messages** and drop oldest only for **live transient** data on slow Bridge, counted in `live_dropped`; no synthetic backfill on reconnect. Newly reconnected subscriber receives fresh samples, not the historical backlog. Heartbeat initial 15s, disconnected timeout initially 45s; exact values subject to shared Bridge acceptance. Send events in queue order and expose gaps via sample ID/sequence. Live delivery status never sets SQLite `saved` or `persisted_at`.

## 6. Final-only HTTPS and immutable durable ACK

**Submission** (proposed `POST /v2/agents/aggregates`):

```json
{
  "type": "final_aggregate",
  "protocol_major": 2,
  "schema_version": "2.0",
  "agent_id": "<stable-agent-id>",
  "record_id": "<deterministic(agent_id,UTC-start,900)>",
  "period_start_utc": "2026-10-18T12:00:00Z",
  "period_seconds": 900,
  "final": true,
  "coverage": {"expected_slots": 900, "observed_slots": 820},
  "sampling_profile_version": 1,
  "metrics": {
    "cpu": {"cadence_seconds": 1, "scheduled": 900, "valid": 810,
            "coverage": {"numerator": 810, "denominator": 900},
            "peak_kind": "actually_measured"},
    "load": {"cadence_seconds": 5, "scheduled": 180, "valid": 160,
             "coverage": {"numerator": 160, "denominator": 180}},
    "filesystem_capacity": {"cadence_seconds": 30, "scheduled": 30,
                            "valid": 28,
                            "coverage": {"numerator": 28, "denominator": 30}}
  },
  "quality": {"metric_quality_version": 1, "partial": true},
  "payload_sha256": "<SHA256 of approved canonical payload bytes>"
}
```

**Canonical hash definition (engineering-selected, external approval pending):** build the immutable submission object with all required identity, period, schema, quality and metrics fields **but omit only the `payload_sha256` member**; serialize it to canonical UTF-8 JSON following **RFC 8785 (JCS)**, then compute SHA-256 of those canonical bytes. Reject NaN/Infinity and invalid/non-representable numbers. Add `payload_sha256` to the envelope and store the **exact transmitted JSON bytes** in SQLite `wire_payload` for byte-identical retries. The transmitted `wire_payload` **is not itself hashed** because it contains its own digest. API mocks and central producer/consumer must use the same normative fixture and canonicalization; different hash for same logical `record_id` is a permanent conflict. Store/keep old v2 wire payload while pending across upgrades; do not regenerate it under a new schema.

**Canonical coverage semantics:** `coverage.expected_slots` / `observed_slots` describe the 1Hz **frames**; all metric-specific denominators and measured timestamps, freshness-valid duration, missing/error/reset/status and entity-level quality must be encoded in `metrics` / `quality` and included in canonical hash. The illustrative metrics' valid-counts are independent; do not assume every valid metric coincides with a valid frame or that a 30s collector saw a sub-30s spike. Cached Live display values are **never** uploaded as newly measured historical values. The central API and Historical UI must preserve and correctly interpret this data-quality distinction.

**Numeric precision for JSON/JCS:** represent large unsigned Linux counters (e.g., rx/tx bytes and packet totals) as exact unsigned **decimal strings** in wire metrics to avoid IEEE-754 precision loss beyond 2^53−1. Validate the decimal grammar/range and define signedness/units in schema; bounded floats may represent percentages and load. Preserve this in canonical hash fixtures, including values exceeding 9,007,199,254,740,991. This applies to the external Bridge/API contracts as well.

**ACK example**:

```json
{
  "type": "durable_ack",
  "protocol_major": 2,
  "agent_id": "<stable-agent-id>",
  "record_id": "<same-record-id>",
  "period_start_utc": "2026-10-18T12:00:00Z",
  "period_seconds": 900,
  "payload_sha256": "<matching-hash>",
  "persistence_domain": "primary",
  "durably_committed": true,
  "committed_at_utc": "<trusted-server-commit-time>"
}
```

Agent must validate: authenticated and authorized TLS peer; same registered `agent_id`, period length/start, `record_id`, payload hash, schema-compatible ACK and verified domain; `durably_committed === true` accompanied by **server-side audited guarantee** of durable idempotent PostgreSQL commit. `HTTP 200`, TCP send success, signed-looking JSON on unauthorized transport or Live ACK alone **never** persist locally. `persisted_at_ms` is set atomically to the UTC time the valid ACK is processed by Agent.

**Idempotency:** deterministic ID derived from Agent identity+UTC quarter+resolution; same ID + identical payload after restart or lost ACK must yield one central logical row; same ID/different hash returns non-retryable conflict. Central dedup state or equivalent immutable uniqueness must outlive the primary 30-day 15m storage expiry so 61-day replay cannot silently reintroduce duplicates. If old 15m arrives after central compaction, API must durably incorporate/recompute hourly/daily summaries before issuing an ACK. These are **external owner commitments**; mocked tests do not prove production central compliance.

**HTTP/error classification**:

| Case | Agent outcome |
|---|---|
| 2xx without valid durable ACK | retain pending, emit protocol error |
| Timeout/reset/5xx | bounded randomized exponential retry |
| 429 | honor bounded authenticated `Retry-After`; no hot loop |
| 401/403, revoked or invalid certificate | `AUTH_BLOCKED`; no indiscriminate retries |
| Unsupported schema, same-ID/conflicting hash | quarantine pending/alert; never overwrite finalized row |
| Unknown agent or ACK from wrong domain/peer | reject, `persisted_at=NULL` |
| Commit succeeded but ACK lost | resend identical immutable bytes; central dedup -> valid ACK |
| Persisted locally before restart | exclude from pending selector; retain until 60-day expiry |

An in-flight submission is transient in RAM only; after restart the SQLite `persisted_at IS NULL` predicate defines pending state. Initial max in-flight one and oldest-first stable order. No database write transaction held during HTTP.

## 7. N-endpoint selection, failover and authority

Per channel select the **first** index in `monitoring_servers` satisfying reachability, supported channel, successful authentication, explicit authorization and safety conditions. Exactly one active endpoint per channel; no broadcast/fanout of identical Persistence finals to unrelated stores. Probe budgets do not scale to unbounded active sockets across N.

- Initial randomized exponential backoff engineering window **1–60s**, per endpoint/channel, with jitter, deadlines and bounded retries.
- Failover from failed endpoint to next **authorized eligible** endpoint, with distinct `ACTIVE` connection per channel.
- Higher-priority failback only after sustained healthy probe (initial engineering default >=90s), with old socket drained/canceled so no two simultaneous authoritative active deliveries.
- Persistence endpoint is eligible **only if** same *verified* durable domain and central dedup scope as currently authorized final record destination, or another explicitly approved cross-domain replication contract. A mere common name `primary` does not prove this.
- Live can independently choose a different host, even one whose Persistence has an unrelated storage domain.
- Reload revoke/reorder cancels revoked authority promptly and cannot retroactively accept an ACK from an endpoint no longer authorized for that record/domain.

## 8. Threat, privacy and operational controls

| Threat | Required mitigation / test |
|---|---|
| MITM, wrong server name, untrusted CA | strict mTLS chain/hostname, no insecure fallback |
| Stolen enrollment token | short expiry, single use, scope and issuer revocation, no persistence in config/log |
| Stolen client key / cloned host | restricted OS permissions, issuer revoke/quarantine, fresh registration, no silent ID reset |
| Valid cert unauthorized channel | separate endpoint/channel/Agent ACL, reject at Hello/send |
| Forged central durable ACK | trusted context, record/hash/domain binding, durable API mock negative fixtures |
| Oversized nested metrics or high cardinality | input cap, bounded allocations and explicit quality/error |
| Config path substitution / symlink | owner/permission validation, safe atomic replacement and reload |
| Clock skew / expired certificates | monitor wall/monotonic disagreement; fail closed auth; clock quality blocks misleading finality |
| Disk-full/corrupt SQLite | no unACKed deletion, explicit failure, operator-controlled restore |
| Denial of service via flapping endpoints | bounded retry jitter/hysteresis and one active/channel |
| Cross-domain replay / stale ACK | verified dedup domain and revocation scope; retain Pending if unsafe |
| Information leakage | no logging credential material; minimal host metrics, structured redaction |

No telemetry side channel or automatic reporting to an unrelated vendor is authorized. All sample and host metadata share the same purpose-limited security boundary as the product.

## 9. Mock contract fixtures and release dependency

Required mock assertions: 1Hz Live frame with correct 1s/5s/30s actual measurement timestamps, not_due Load/Filesystem on off-ticks and safe cached old display; metric-specific coverage 40/60 CPU versus 8/12 Load and normal 2/2 filesystem, old/stale does not count as new, JCS hash unchanged across idempotent retry; incompatible major rejection; bad CA/SNI/expired/revoked cert; valid mTLS but unauthorized Agent/channel; N=1/3/10 ordered selection; Live slow/drop/reconnect with no history persistence; persistence rejects raw/60s; 200 without ACK; forged/wrong hash/wrong domain ACK; PostgreSQL mock commit succeeded/ACK lost/identical replay; two shared versus independent central domains; 61-day late arrival/reaggregation; config revoke during ACK and rotation while live.

**Unresolved external sign-offs (not product option votes):** Bridge ingress/outbound subscriber schema, Persistence ACK exact format and durable implementation, domain proof, central >30d late ingestion and dedup retention, PKI issuer/rotation/revocation/re-enrollment, new Desktop integration using only the new Agent protocol. Any promise absent from other service implementations remains BLOCKED and must not be declared complete by Agent-only tests.

## 10. License, provenance and release authenticity

Current source and distributed v0.1.0 are MIT; Apache-2.0 is the **intended** future license after copyright/third-party rights audit and NOTICE work. Verify signed artifact provenance/identity (e.g. keyless Sigstore/Cosign or equivalent approved mechanism) before unattended installs. Signed checksums complement, not replace publisher authentication. No deployed users or old Desktop consumers exist, so no legacy binary coexistence or compatibility gate is required; deliver only a clean new-Agent installation. No code changes or automated deployment follow merely from this document.

## 11. SSH-only local operator management contract (2026-10-18)

Configuration edit, validation, resource-limit provisioning and storage status/safe-cleanup are **local administrative operations invoked through an existing operator-authorized SSH/Ansible session**. They do not use Live WSS, Persistence HTTPS, the central Bridge, or a new inbound Agent REST/network API. Do not add a resident management server, Agent-owned SSH identity or remote code execution surface. The Desktop supplies UX and existing SSH authorization; the Linux OS controls file ownership, service account privileges, privileged systemd operations and local maintenance permissions.

Configuration replacement is atomic (staged file, validated policy, fsync/rename where supported), with auditable revision and explicit active/effective status. A malformed or unauthorized config must not silently replace the active validated snapshot. Only trusted, authorized operators may apply systemd drop-ins; non-root Agent must not self-elevate. Privileged processes should receive narrowly scoped inputs, never arbitrary shell fragments or SQL from the Desktop.

A proposed local administrative CLI may later support `config validate`, `config status`, `storage status`, `storage cleanup --dry-run` and `storage cleanup --safe`; **names and exact JSON envelope are design candidates**, not implemented commands or approved v2 wire contracts. Operators must see whether CPU/memory quotas are *desired* or verified *effective*. Storage cleanup is ACK/retention/lineage guarded, bounded, auditable, redacted and never deletes unACKed Pending. Host CPU/RAM/disk alert thresholds are Desktop-owned evaluation policy, not permissions to mutate host resources. No enrollment key/token is read into a user-facing diagnostics response.

## 12. v0.3.0 executable wire vs target protocol — integration handoff

**Relay builders start here for executable wire; prefer [MONITORING_RELAY.md](MONITORING_RELAY.md) for a standalone how-to.** Sections 1–11 are the owner-approved **target**. This section states what Agent **v0.3.8** actually sends today.

### Current Live request — implemented Agent output

The Agent connects with `github.com/coder/websocket` to the configured `live_url` using outbound TLS 1.3 minimum, trusted CA, `server_name` verification and configured mTLS client cert/key. After dial it sends `agent_hello`, waits for `hello_accepted` (fail-closed on timeout, wrong identity/channel, unsupported `protocol_major`, or out-of-range `heartbeat_seconds`; bound ≤300s), then streams frames and application heartbeats. Peer silence beyond approximately **3×** `heartbeat_seconds` triggers reconnect. `max_message_bytes` from `hello_accepted` is accepted when present but **not** enforced on outbound writes in this slice.

Hello / accept shapes match §4. Sample frames still use temporary `type: live` (not yet `live_sample`):

```json
{
  "type": "live",
  "agent_id": "opaque-stable-local-id",
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

Application heartbeat (interval from `hello_accepted.heartbeat_seconds`):

```json
{ "type": "heartbeat", "sent_at_utc": "2026-10-18T12:00:00Z" }
```

`frame.metrics` holds only fields the collector emitted on that tick; metric values may be scalar or objects. Status `valid` and `error` are used by the current collector; the richer proposed quality/cadence/freshness envelope is **not** currently serialized. The collector uses 1-second CPU/memory/network/disk probes, 5-second Load and 30-second filesystem probes. The runtime Live buffer is capacity 2 with old-frame drop on pressure. Relays **must** accept Hello then `type: live` (or provide an explicit temporary adapter). A Relay that only understands proposed `live_sample` without Hello **will not interoperate as-is**. Desktop subscriber protocol and registrar ACL beyond Hello identity checks remain external.

**Relay ingress implementation dependency:** jointly specify strict message schema, bounded message size, host/Agent registration ACL, negotiated version and heartbeat, duplicate/gap semantics, and temporary compatibility for `type=live` after Hello. Label that compatibility an explicitly tested *temporary* adapter, not an undocumented production contract. Relay must relay to authenticated Desktop subscribers, not store monitoring history. Subscriber URLs/handshakes and authorization are unverified and belong to the Relay/Desktop implementations.

### Current Persistence request and ACK — implemented Agent behavior

Only final local `resolution_seconds=900` aggregates with `persisted_at_ms IS NULL` are fetched oldest-first. The Agent POSTs the exact saved `wire_payload` bytes with `Content-Type: application/json` and `Idempotency-Key: <record_id>` to the configured `persistence_url` over HTTPS/mTLS. The actual stored JSON is built by `storage.Final` and looks like this structurally (timestamps, IDs and checksum are illustrative):

```json
{
  "agent_id": "opaque-stable-local-id",
  "schema_version": 2,
  "period_start_ms": 1792324800000,
  "resolution_seconds": 900,
  "record_id": "stable-record-id",
  "sample_count": 840,
  "expected_slots": 900,
  "observed_slots": 840,
  "summary": {
    "cpu.total_pct": {
      "count": 840, "due": 840, "failed": 0,
      "sum": 10500, "min": 2, "max": 80,
      "last": 12, "peak_at_ms": 1792324890000,
      "last_at_ms": 1792325600000
    }
  },
  "payload_sha256": "hexadecimal-sha256"
}
```

**Important gap:** the current payload digest is computed using Go `json.Marshal` of the struct without `payload_sha256`, **not an RFC 8785 JCS canonicalization implementation**. Do not claim current code satisfies the target hash specification in section 6. Schema fields such as per-metric scheduled coverage, quality and original freshness are not yet fully represented in this actual payload.

The current code accepts **only HTTP 200** and decodes an ACK with these exact JSON keys:

```json
{
  "durable": true,
  "agent_id": "opaque-stable-local-id",
  "record_id": "stable-record-id",
  "payload_sha256": "matching-hexadecimal-sha256",
  "period_start_ms": 1792324800000,
  "persistence_domain": "primary",
  "schema_version": 2
}
```

The Agent checks `durable=true` and exact equality of `agent_id`, `record_id`, `payload_sha256`, `period_start_ms`, `persistence_domain` and `schema_version=2` before marking its row acknowledged. Current code does not carry a signed independent database proof; **actual durability and registration authorization are obligations of the authenticated central API and still require external signoff**. A returned `200` without matching ACK never advances `persisted_at`. For a lost ACK the Agent retries the identical stored bytes; the central service must enforce uniqueness/idempotence by record ID and hash.

The section-6 proposed `durable_ack` envelope (including `type`, `period_start_utc`, `committed_at_utc`) is **not** the v0.3.0 decoder's format. Central API owners must choose and approve a single interoperable contract and write cross-repository fixtures/tests before treating the final release as accepted.

### Current failover and connection limitations

- Live implements authorized Hello + application heartbeats, then cycles eligible endpoints after connection/stream errors (~3s delay). It does **not** implement priority-based hysteresis/failback or a local registrar ACL (ACL is enforced by the Relay/registrar).
- Persistence selects the first configured Persistence URL and retries there every ~5 seconds; **no persistence failover** currently occurs even when more URLs are configured.
- No hot config reload, certificate enrollment/renewal/revocation automation, active channel status query or Desktop provisioning API currently exists.
- A `persistence_domain` string in config and a matching ACK field are **not proof** of shared authoritative durable storage. Do not fail over across domains until that guarantee is established.
- An operational model with **one active Live Relay per Agent** is compatible with the target; do not equate an ordered endpoint list with concurrent active relays.

### Minimum cross-component contract tests required before integration approval

| Test | Expected outcome |
|---|---|
| Unregistered Agent with otherwise valid mTLS certificate | Server denies ingestion by channel/Agent ACL |
| Registered Agent Live frame | Relay accepts the agreed version/schema and forwards to only authorized subscribed Desktop clients |
| Agent Live connection drops | Desktop displays stale/disconnected state; no historical replay through Relay |
| Persistence ACK has wrong record/hash/domain or only HTTP 200 | Local final remains Pending |
| Central commit succeeds but ACK is lost | Exact replay is deduplicated centrally and eventually valid ACK is produced |
| Central API receives raw or 60-second aggregation | Reject; only immutable 900-second final is allowed |
| More than one configured endpoint | Only one Live active at a time; Persistence failover remains disabled until verified shared-domain implementation exists |
| Desktop changes Agent settings | Existing authorized SSH path with validate, controlled restart and rollback; no inbound Agent API |
| Desktop reads historical charts | Requests Historical API, not Live Relay nor direct Agent SQLite |
| Certificate expires/revokes / host cloned | No unauthorized connection; explicit re-enrollment or quarantine by external authority |

**Release impact:** This section is the implementer-facing reality check for v0.3.0. The target section-4/5/6 envelopes remain proposed. They must not silently be presented as implemented; discrepancies are integration blockers, not mere wording differences. For the executable local setup sequence see [Configuration](../CONFIGURATION.md).
