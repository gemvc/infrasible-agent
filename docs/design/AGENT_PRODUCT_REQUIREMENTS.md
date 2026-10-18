# Infrasible Monitoring Agent — Product Requirements

**Status:** PRODUCT / ARCHITECTURE BASELINE APPROVED IN DESIGN DISCUSSION (2026-10-18); evidence-based G00 gate and external interface sign-offs remain pending.
**Scope:** standalone, open-source Linux Agent in `gemvc/infrasible-agent`. **Source baseline:** `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed` (current source is legacy v0.1.0).
**Change authority:** owner explicitly approved DEC-01–11 and the reviewed engineering resolutions on 2026-10-18, then authorized repository/backlog documentation updates. This does **not** authorize runtime implementation, changes to Transible Desktop/SSH, other services, production deployments, releases, or a license change without legal clearance.

## 1. Outcome, users and operational journeys

A small Linux daemon collects host CPU, memory/swap, load, filesystem and network metrics at nominal 1 Hz; durably stages raw observations, produces UTC-aligned one-minute and finalized fifteen-minute summaries, emits fresh live samples to a stateless bridge, and uploads only final fifteen-minute records to a distinct durable Persistence API. The Agent can be installed by Transible through existing SSH or provisioned independently; it never polls SSH per second.

- **Operators:** register a host, see near-real-time Live UI while the Bridge is reachable, distinguish stale/degraded from healthy, diagnose gaps and Pending history, recover a crashed agent or revoked certificate safely.
- **Transible Desktop:** use its existing SSH implementation only for initial install/registration and explicit administrative actions; Live UI subscribes to Bridge; Historical UI queries central Historical API.
- **Central service operators:** enforce registered Agent identities and channel privileges; Live Bridge relays ephemeral events without history; Persistence API commits immutable 15m records durably/idempotently in PostgreSQL and returns authenticated durable ACKs; historical retention/late-arrival semantics belong to central owners.
- **Open-source independent operator:** perform provisioning and recovery without requiring the proprietary Desktop code or credentials in the Agent repository.

## 2. Scope and exclusions

**In:** Linux collectors, 1Hz scheduler, bounded quality-aware sampling, single local SQLite, 1m and 15m rollups, 60-day acknowledged final retention, indefinite pending retention while capacity permits, outbound Agent-initiated Live WSS and Persistence HTTPS with mTLS, one ordered versioned N-endpoint config, domain-safe per-channel failover, registration/rotation interface, systemd unattended service, health, secure signed packages, migration documentation, tests and mock contracts.

**Out:** Desktop UI/SSH core/vault alterations, shipping Live Bridge or PostgreSQL/Persistence/Historical API here, under-live tracking, direct database writes, inbound public metrics server, Redis/JSONL/queue database/fourth application table, guaranteed lossless live event delivery, historical replay via Bridge, automatic destructive recovery, unconditional Apache-2.0 relicensing, automatic production deployment.

## 3. Owner-approved decision register

| Decision | Approved resolution | Qualification still requiring verification |
|---|---|---|
| DEC-01 | Persistent outbound **WSS + mTLS** for Live; **HTTPS keep-alive + mTLS** for Persistence; independent channels | External envelope, Hello/ACK/schema compatibility tests |
| DEC-02 | Existing SSH provisions/registers initially; one stable `agent_id`; ongoing Agent-initiated mTLS | Enrollment issuer/authorized domain/cert lifecycle contract |
| DEC-03 | UTC one-minute grace **120s after minute end**, then immutable | Clock-step/late-arrival and crash tests |
| DEC-04 | Acknowledged final 15m rows expire after **60 days**; unacknowledged rows survive beyond 60 days until a valid ACK | Explicit disk-full/degraded and operator recovery contract; no infinite physical-storage claim |
| DEC-05 | Persistence failover only among endpoints with **verified shared persistence/dedup domain**; Live failover independent | Domain assertion must be verified, not merely config text |
| DEC-06 | **Pure Go SQLite, WAL, synchronous=FULL, one controlled writer** | Exact library chosen after native ARM/AMD benchmark and failure tests |
| DEC-07 | **Daemon-only new Agent**; old `version/snapshot/stream --json` public commands removed | Versioned/bounded Desktop consumer migration; no silent in-place replacement |
| DEC-08 | Peak **RSS <= 128 MB**; mean **CPU <= 5% of one core**; sampling schedule jitter **P99 <= 500 ms** | Definitions, reference rigs, cardinality, disk and replay benchmark below |
| DEC-09 | Apache-2.0 intended **after copyright/legal audit**, proper NOTICE, signed/verifiable release artifacts | Existing repository remains MIT until audit and explicit legal action |
| DEC-10 | **Controlled fail-closed corruption recovery**; same-host restore preserves identity; clones require distinct identity | Clone assurance and Registration revocation policy |
| DEC-11 | `server_info` in SQLite stores one host/Agent identity; ordered N Monitoring endpoints in **one versioned config** | Atomic config reload, separate endpoint/channel authority |

Approved engineering defaults below may be tuned only with a recorded evidence-backed ADR **without changing any DEC invariant**. Exact external API commitments require their service owner's sign-off.

## 4. Locked correctness requirements

| Requirement | Guarantee |
|---|---|
| FR-01 | Exactly three logical application tables `server_info`, `raw_samples`, `aggregates` in a single local SQLite database; no queue table. SQLite internal tables are not additional application tables. |
| FR-02 | Capture each actual sample approximately every second with measured elapsed time, stable sample ID, UTC observation time and explicit quality; no fabricated values on failed reads, missed ticks, host reconfiguration or clock anomaly. |
| FR-03 | One-minute rollups cover `[minute_start, minute_end)`; finalize no sooner than end plus 120s grace; immutable thereafter. |
| FR-04 | `raw_samples.saved=true` only **in the same committed transaction** as its exact minute aggregate; no unsaved raw cleanup. |
| FR-05 | Fifteen finalized minute periods yield one immutable fifteen-minute record by an idempotent transaction; represent missing coverage, no synthetic samples, no source cleanup before destination commit. |
| FR-06 | Preserve valid per-metric count, elapsed-time-weighted average when appropriate, min/max/last, peak magnitude/time, anomaly and observed-versus-expected coverage (entity-specific where necessary). |
| FR-07 | Live publishes fresh raw data only to an authenticated, authorized stateless Bridge. It does not block sampler/SQLite/Persistence; bounded live buffering permits counted drops. |
| FR-08 | Persistence sends **only finalized 900s records** oldest-pending-first; `persisted_at` changes only after a verified authenticated ACK bound to matching ID/hash/agent/domain after central durable commit. |
| FR-09 | Same ID + identical immutable payload on retries, including ACK-lost/commit-succeeded; the central domain must deduplicate and reject same-ID/different-payload. |
| FR-10 | One active endpoint **per channel**, priority by array index, independently eligible; never fail over Persistence to an unrelated unverified domain; bounded reconnect and hysteresis. |
| FR-11 | Initial SSH provisioning does not alter Desktop core SSH; Agent owns no SSH credentials; subsequent operation is independent of SSH. |
| FR-12 | Fail closed on missing/invalid identity, revoked trust, SQLite corruption, unsafe downgrade or unresolved host clone; preserve diagnostics without secret disclosure or silent data deletion. |
| FR-13 | ACKed final rows locally retained for 60 days and then cleanup-eligible; older unACKed final records are retained until ACK (subject to explicit observable storage failure). |
| FR-14 | New public service interface is daemon-only and protocol-major 2 design; legacy v0.1.0 stays an archived supported-to-migrate artifact, never invisibly overwritten for v1 CLI consumers. |
| FR-15 | No external API's advertised availability/ACK/history behavior is inferred from Agent mock implementation; external owners must accept it. |

## 5. Lifecycle, trust and visible status

Service states: `STARTING`, `HEALTHY`, `LIVE_DEGRADED`, `PERSISTENCE_DEGRADED`, `STORAGE_DEGRADED`, `CLOCK_UNTRUSTED`, `AUTH_BLOCKED`, `RECOVERY_REQUIRED`, `STOPPING`. They are **orthogonal health dimensions**, not one exclusive enum: Live may be down while Persistence remains healthy; health reflects last successful raw commit, finalized minute/quarter, last authenticated ACK, oldest pending age/count, retry errors, WAL/DB disk health and clock confidence. Central consumers infer Stale/Offline from last received heartbeat; no impossible Agent push is claimed during total disconnection.

Agent service target: non-root account supervised by systemd on supported distributions; config `/etc/infrasible-agent/config.json`, SQLite `/var/lib/infrasible-agent/agent.db`, protected client certificate/key in the Agent-owned state directory; structured redacted journald diagnostics; **no public inbound listener**. Physical path and ownership may be adjusted for packaging before G10 if tested and documented.

## 6. SLO and measurement contract

- **Nominal schedule:** one collection initiation per 1000ms on monotonic time; no catch-up synthetic snapshots. Jitter is actual start minus planned monotonic tick. P99 <= 500ms in approved reference steady workload; record actual interval for rate calculations.
- **CPU:** mean process CPU time / elapsed wall time <= 0.05 CPU-seconds/second (5% of ONE core), not 5% total host CPU; include normal collection/rollup and healthy dual-channel transport.
- **RAM:** peak resident set size (RSS) <= 128 MB for qualified normal/reference cardinality workload, not merely Go heap. Measure startup, one-hour steady state, reconnect, WAL checkpoint and catch-up separately; abnormal high-cardinality load must be bounded and explicitly described.
- **Disk:** no arbitrary universal quota. Benchmark measured serialized entity cardinality, `raw_samples` staging, one-minute staging, 5,760 acknowledged 15m final records/60d, SQLite indexes and WAL checkpoint, extra unACKed backlog over 60d. At 16KiB/final, payload-only ~90MiB; this is **illustrative**, not a budget.
- **Replay:** engineering qualification target >= 2 final records/s on a healthy local mock endpoint while maintaining 1Hz cadence; downgrade to measured/owner-approved value via ADR if hardware cannot meet it. At 61d × 96 final/day = 5,856 records, 2/s implies ~49min ideal; real network and API rate limits matter.
- **Platforms:** attempt cross-build Linux amd64/arm64/armv7/armv6. Native runtime security/durability/performance qualification, not cross-compilation, determines whether each can be advertised fully supported; unqualified platforms must be marked experimental/excluded.
- **Reference harness:** pinned OS/kernel, CPU/RAM/architecture, process limits, mount count/interfaces, disk medium, SQLite driver/version, network RTT/throughput, sample payload size, tests/commands and raw measurements. `P99` measured on >= one-hour steady-state fixture with explicit warm-up, denominator and reproducible seed; outage/replay scenarios reported separately.
- **No false guarantee:** data from failed collection, stopped host, ENOSPC and unavailable central services cannot be guaranteed lossless. The requirement is explicitly visible degradation and preservation of available committed records.

## 7. Dependency and sign-off boundaries

| Dependency | Required contract | Owner / evidence |
|---|---|---|
| Live Bridge | Authorized WSS agent ingress, subscriber fanout, heartbeat, no durable history or historical query | Live/bridge interface owner and mock fixture |
| Persistence API/PostgreSQL | 900s final-only, immutable-ID idempotent durable commit, same-ID/hash semantics, trusted domain and authenticated durable ACK, late arrival >30 days, safe retention/dedup tombstones | Central persistence/data owner and mock fixture |
| Historical API | PostgreSQL-backed reads only; 15m ~30d, hourly ~1y, daily longer-term and late-arrival reaggregation | Central historical owner |
| Enrollment/PKI | Short-lived authorized bootstrap, Agent-generated private key, CSR/cert binding, rotation, revocation, channel ACL and clone remediation | Security/registration owner |
| Transible consumer | Explicit migration off v1 stdout CLI before any new binary takes its place, **without changing Desktop SSH core in this repo** | Desktop integration owner |
| License/security | Copyright ownership, compatible third-party notices, signed verifiable release and revocation procedure | Legal/release owner |

## 8. Source inventory and source-of-truth

As-built at baseline: `main.go` (snapshot and CLI stdout), `version.go`, `main_test.go`, `version_test.go`, `go.mod` (Go 1.23, no third-party modules), `docs/PROTOCOL.md` (v1 stdout/SSH), `docs/INSTALLATION.md`, `scripts/package-linux.sh`, CI/release workflows, MIT `LICENSE`. v0.1.0 was released 2026-10-17; README still describes it as not yet released. The new architecture is **not currently implemented**. Read historical protocol files as AS-IS, never as overriding TO-BE.

Canonical split: this file (product/DEC/SLO), `AGENT_ARCHITECTURE.md` (state and boundaries), `AGENT_SQLITE_SCHEMA.md` (DB), `AGENT_PROTOCOL_AND_SECURITY.md` (wire/auth/HA), `AGENT_ACCEPTANCE_AND_TEST_MATRIX.md` (proof), and the existing `AGENT_IMPLEMENTATION_BACKLOG.md` (single implementation plan). Do **not** add a competing implementation plan.

**G00 rule:** Owner has approved the architectural choices and documentation work, **not** code. Close G00 only when these five documents, the updated backlog, schema/wire/mock and external-interface review evidence meet the gate and all required reviewers sign. Specific driver/native performance and legal migration remain gated by later qualification or legal review.
