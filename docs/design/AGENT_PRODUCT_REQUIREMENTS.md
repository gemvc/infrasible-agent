# Infrasible Monitoring Agent — Product Requirements

> **Greenfield product baseline (approved 2026-10-18):** This is a new product with zero users, zero deployments and zero production data. Implement a clean-install daemon only. The historical `v0.1.0` CLI and its release are reference material, not supported consumers or migration targets. No backward compatibility, coexistence, old-database import, pre-release upgrade/downgrade or old-client test gate is required. Keep fresh-schema version validation, crash-safe restart, same-host recovery, identity/certificate rotation, security and release tests. Future post-launch upgrades are outside the initial release. This decision is canonical in this document.

**Status:** FINAL OWNER-APPROVED PRODUCT DESIGN (2026-10-18), including independent metric cadences and first-class quality/freshness. G00 evidence and external-interface sign-offs remain pending; no runtime implementation is claimed.
**Scope:** standalone, open-source Linux Agent in `gemvc/infrasible-agent`. **Source baseline:** `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed` (current source is legacy v0.1.0).
**Change authority:** owner explicitly approved DEC-01–11 and the reviewed engineering resolutions on 2026-10-18, then authorized repository/backlog documentation updates. This does **not** authorize runtime implementation, changes to Transible Desktop/SSH, other services, production deployments, releases, or a license change without legal clearance.

## 1. Outcome, users and operational journeys

A small standalone Linux daemon runs a nominal **1 Hz frame/scheduler** with **independent per-metric collection cadences**, durably stages real observations with their measurement time/quality/freshness, makes UTC-aligned 1m and 15m summaries with per-metric coverage, sends fresh Live frames to a stateless Bridge, and uploads only immutable final 15m to a distinct Persistence API. The Agent can be installed by Transible through existing SSH or provisioned independently; it never polls SSH per second.

- **Operators:** register a host, see near-real-time Live UI while the Bridge is reachable, distinguish stale/degraded from healthy, diagnose gaps and Pending history, recover a crashed agent or revoked certificate safely.
- **Transible Desktop:** use its existing SSH implementation only for initial install/registration and explicit administrative actions; Live UI subscribes to Bridge; Historical UI queries central Historical API.
- **Central service operators:** enforce registered Agent identities and channel privileges; Live Bridge relays ephemeral events without history; Persistence API commits immutable 15m records durably/idempotently in PostgreSQL and returns authenticated durable ACKs; historical retention/late-arrival semantics belong to central owners.
- **Open-source independent operator:** perform provisioning and recovery without requiring the proprietary Desktop code or credentials in the Agent repository.

## 2. Scope and exclusions

**In:** Linux collectors, 1Hz frames with per-metric scheduling and bounded collection, explicit measured-at/quality/freshness, one local SQLite, metric-quality-preserving 1m and 15m rollups, 60-day acknowledged final retention, indefinite pending retention while capacity permits, outbound Agent-initiated Live WSS and Persistence HTTPS with mTLS, one ordered versioned N-endpoint config, domain-safe per-channel failover, registration/rotation interface, systemd unattended service, health, secure signed packages, clean-install documentation, tests and mock contracts.

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
| DEC-07 | **Daemon-only greenfield Agent**; old `version/snapshot/stream --json` commands are not supported | No legacy user/client migration or backward-compatibility obligation; clean installation only |
| DEC-08 | Peak **RSS <= 128 MB**; mean **CPU <= 5% of one core**; sampling schedule jitter **P99 <= 500 ms** | Definitions, reference rigs, cardinality, disk and replay benchmark below |
| DEC-09 | Apache-2.0 intended **after copyright/legal audit**, proper NOTICE, signed/verifiable release artifacts | Existing repository remains MIT until audit and explicit legal action |
| DEC-10 | **Controlled fail-closed corruption recovery**; same-host restore preserves identity; clones require distinct identity | Clone assurance and Registration revocation policy |
| DEC-11 | `server_info` in SQLite stores one host/Agent identity; ordered N Monitoring endpoints in **one versioned config** | Atomic config reload, separate endpoint/channel authority |

Approved engineering defaults below may be tuned only with a recorded evidence-backed ADR **without changing any DEC invariant**. Exact external API commitments require their service owner's sign-off.

## 4. Locked correctness requirements

| Requirement | Guarantee |
|---|---|
| FR-01 | Exactly three logical application tables `server_info`, `raw_samples`, `aggregates` in a single local SQLite database; no queue table. SQLite internal tables are not additional application tables. |
| FR-02 | Commit one actual one-second **frame** with stable ID, UTC frame time and real elapsed interval. Different metric families run on their approved schedule; each actual metric measurement has independent `measured_at_utc`, unit, status and quality. Do not manufacture a fresh observation when a collector is not due or fails. |
| FR-03 | One-minute rollups cover `[minute_start, minute_end)`; finalize no sooner than end plus 120s grace; immutable thereafter. |
| FR-04 | `raw_samples.saved=true` only **in the same committed transaction** as its exact minute aggregate; no unsaved raw cleanup. |
| FR-05 | Fifteen finalized minute periods yield one immutable fifteen-minute record by an idempotent transaction; represent missing coverage, no synthetic samples, no source cleanup before destination commit. |
| FR-06 | Preserve **per-metric scheduled opportunities, attempts, valid/invalid/missed observations, last measured time, age/freshness, count, coverage**, and meaningful weighted average, measured min/max/last, peak value/time and anomaly evidence in 1m and 15m rollups; frame-level 60/900 slots do not substitute for per-metric expected opportunities. |
| FR-07 | Live publishes fresh raw data only to an authenticated, authorized stateless Bridge. It does not block sampler/SQLite/Persistence; bounded live buffering permits counted drops. |
| FR-08 | Persistence sends **only finalized 900s records** oldest-pending-first; `persisted_at` changes only after a verified authenticated ACK bound to matching ID/hash/agent/domain after central durable commit. |
| FR-09 | Same ID + identical immutable payload on retries, including ACK-lost/commit-succeeded; the central domain must deduplicate and reject same-ID/different-payload. |
| FR-10 | One active endpoint **per channel**, priority by array index, independently eligible; never fail over Persistence to an unrelated unverified domain; bounded reconnect and hysteresis. |
| FR-11 | Initial SSH provisioning does not alter Desktop core SSH; Agent owns no SSH credentials; subsequent operation is independent of SSH. |
| FR-12 | Fail closed on missing/invalid identity, revoked trust, SQLite corruption, unsupported on-disk schema or unresolved host clone; preserve diagnostics without secret disclosure or silent data deletion. |
| FR-13 | ACKed final rows locally retained for 60 days and then cleanup-eligible; older unACKed final records are retained until ACK (subject to explicit observable storage failure). |
| FR-14 | New service is daemon-only with a new explicitly versioned network protocol; old v0.1.0 is historical reference only, not a supported migration artifact or compatibility target. |
| FR-15 | No external API's advertised availability/ACK/history behavior is inferred from Agent mock implementation; external owners must accept it. |
| FR-16 | Explicitly distinguish actual `valid` (including true zero), due-but-`missing`, read `error`, `counter_reset`, age-`stale`, `not_due` and `unsupported`. `null`/unknown never equals measured zero, and `stale` never counts as a new observation. |
| FR-17 | A Live view may hold an old value only with original `measured_at_utc`, age/TTL and old-value status; cached values cannot inflate local raw or aggregate sample counts. After startup absent metrics are unknown until measured. |
| FR-18 | Keep small modular Go collectors inside **one daemon binary**; no general plugin runtime, AI engine, in-Agent dashboard, public inbound REST port, scripts or server-configuration automation. Ship meaningful per-channel and per-metric health in the first vertical slice. |

## 4A. Final owner-approved engineering refinements — 2026-10-18

These refinements supplement **rather than supersede** DEC-01–11. The DEC-03 **UTC minute grace remains 120 seconds**; an earlier hypothetical shorter grace was not approved. Sampling intervals are **default normative policies**, only adjustable via a versioned/validated sampling profile and matching coverage/performance tests.

### AD-01 — A 1-second raw frame with independent metric cadences

| Metric | Default cadence | Exact meaning |
|---|---:|---|
| CPU utilization/CPU counters | 1 second | Actual `/proc/stat` interval; avoid duplicate guest accounting |
| Network interface counters, bytes/packets/errors/drops and derived rates | 1 second | Counter delta / observed monotonic elapsed; invalid resets reported |
| Memory and swap | 1 second | Genuine point-in-time bytes; zero only if source read succeeds |
| Disk I/O counters/rates | 1 second | Linux block I/O metrics where available, with explicit device identity/quality; **distinct** from filesystem free capacity |
| Load averages, runnable/total tasks | 5 seconds | Already rolling-average metrics; retain original measurement timestamp |
| Filesystem space/capacity per mount | 30 seconds | Mount-level `statfs` with source time, mount churn/errors |
| Mostly static OS/kernel/hardware/host metadata | startup and detected change | Event-driven or bounded low-frequency reconciliation when needed; never scan all metadata every 1s |

One actual Raw frame each nominal second remains the **only raw staging row**: it carries actual newly collected readings and due collector errors, not fabricated repeats of slower measurements. `not_due` may be represented by omission plus the approved sampling profile. A small bounded in-memory last-valid cache may support Live presentation, but each reused value remains clearly **old**, is never re-stamped as measured now, and never creates a new raw observation. All schedules use a common monotonic tick source and bounded collector work with no per-metric goroutine explosion.

New metric types must declare units, source, cadence, stable entity identifiers, quality/freshness, per-metric aggregation math, resource budget and mock/native tests. Unsupported Disk I/O on a platform is `unsupported`, never a measured zero.

### AD-02 — First-class quality, freshness and metric-specific coverage

- `valid`: actual accepted measurement at `measured_at_utc`, including legitimate zero.
- `not_due`: no collection scheduled on this frame; **not** a failed or missing observation.
- `missing`: scheduled reading or frame was absent; counts against expected opportunities.
- `error`: due read/parse/permission failed; typed redacted diagnostic, no valid value.
- `counter_reset`: newly observed counter could not yield a valid rate for this interval; may reset baseline.
- `stale`: previously valid cached value is older than its **metric-specific freshness TTL**; carries old timestamp and is never a new observation.
- `unsupported`: kernel/source does not expose the metric; not confused with transient failure.

Default freshness TTL is **2 × configured cadence + allowed scheduler jitter margin**, explicitly bounded and versioned per metric. The cache may expose `last_valid_at_utc`/age to Live but does not replace invalid/absent readings. A cached sample can be *not newly measured* even when still within TTL: `freshness` and `observation_status` must be independent.

On a full uninterrupted UTC minute, expected nominal opportunities are 60 CPU/Network/Memory/Disk-I/O observations, **12** Load observations and **2** Filesystem-capacity observations. A quarter has 900, 180 and 30 respectively. Startup, downtime, phase alignment and clock anomaly may change the actual due/attempted values; retain explicit profile and due/gap counts rather than falsely assigning a full observation to an offline host. The global `expected_slots` and `observed_slots` in `aggregates` describe **1Hz frame availability only**, not every metric's coverage. Per-metric actual expected/due/valid/error and per-entity coverage belong in `summary_json`/`quality_json`.

Example in one minute: CPU **40/60** valid and Load **8/12** valid are distinct coverages; on healthy 5s Load, four intervening 1s frames without Load are `not_due`, not missing. A 30s Filesystem check can only report **measured** min/max/peak; no claim to detect changes between checks. A duration-weighted gauge average must be calculated only over explicit, bounded **freshness-valid intervals**, documented as last-observation-held estimate if applied (not new observations); never carry old values across stale/untrusted gaps. For counters, use legitimate monotonic delta/time intervals and never average absolute byte totals as though rates.

### AD-03 — Simplicity, early health and vertical-slice delivery

One non-root **Go daemon**, modular built-in collectors, one controlled SQLite writer, two independent bounded Live/Persistence workers. Explicitly **exclude** general plugins, Redis, brokers, separate agent-side DBs, public REST listener, embedded dashboards, AI, arbitrary scripts and automated mutation of monitored servers.

Start the first reviewable vertical slice with actual **CPU + Memory + Network** -> 1s raw frames -> SQLite -> 1m -> 15m -> restart recovery and per-metric quality/health. Then add durable Persistence mock with lost-ACK proof, Live WSS with bounded drops, remaining cadences/metrics, issuer/mTLS, N-endpoint HA, packaging and native qualification in existing P01–P11 dependency gates. This is incremental execution of the **existing single backlog**, not a new roadmap or permission to bypass P00/security prerequisites.

A running process may still be STORAGE_DEGRADED, Live may be current while Persistence is 61 days behind, and a 1s frame may contain 30s-old capacity data. Report collector lag, last valid per metric, last due/failed opportunity, metric age, DB commit health, pending backlog, Live and Persistence health **separately**. Favor truthful degradation over fake success.


## 5. Lifecycle, trust and visible status

Service states: `STARTING`, `HEALTHY`, `LIVE_DEGRADED`, `PERSISTENCE_DEGRADED`, `STORAGE_DEGRADED`, `CLOCK_UNTRUSTED`, `AUTH_BLOCKED`, `RECOVERY_REQUIRED`, `STOPPING`. They are **orthogonal health dimensions**, not one exclusive enum: Live may be down while Persistence remains healthy; health reflects last successful raw commit, finalized minute/quarter, last authenticated ACK, oldest pending age/count, retry errors, WAL/DB disk health and clock confidence. Central consumers infer Stale/Offline from last received heartbeat; no impossible Agent push is claimed during total disconnection.

Agent service target: non-root account supervised by systemd on supported distributions; config `/etc/infrasible-agent/config.json`, SQLite `/var/lib/infrasible-agent/agent.db`, protected client certificate/key in the Agent-owned state directory; structured redacted journald diagnostics; **no public inbound listener**. Physical path and ownership may be adjusted for packaging before G10 if tested and documented.

## 6. SLO and measurement contract

- **Nominal schedule:** one frame initiation per 1000ms monotonic, independently due 1s/5s/30s metric families and startup/change metadata; no catch-up synthetic measurements. Jitter is actual start minus planned monotonic tick. P99 <= 500ms in approved reference steady workload; record actual interval for rate calculations.
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
| Transible integration | New clean-install integration against the new Agent network contract; **no legacy CLI consumer migration**, and no changes to Desktop core SSH in this repo | Desktop integration owner |
| License/security | Copyright ownership, compatible third-party notices, signed verifiable release and revocation procedure | Legal/release owner |

## 8. Source inventory and source-of-truth

As-built at baseline: `main.go` (snapshot and CLI stdout), `version.go`, `main_test.go`, `version_test.go`, `go.mod` (Go 1.23, no third-party modules), `docs/PROTOCOL.md` (v1 stdout/SSH), `docs/INSTALLATION.md`, `scripts/package-linux.sh`, CI/release workflows, MIT `LICENSE`. v0.1.0 was released 2026-10-17; README still describes it as not yet released. The new architecture is **not currently implemented**. Read historical protocol files as AS-IS, never as overriding TO-BE.

Canonical split: this file (product/DEC/SLO), `AGENT_ARCHITECTURE.md` (state and boundaries), `AGENT_SQLITE_SCHEMA.md` (DB), `AGENT_PROTOCOL_AND_SECURITY.md` (wire/auth/HA), `AGENT_ACCEPTANCE_AND_TEST_MATRIX.md` (proof), and the existing `AGENT_IMPLEMENTATION_BACKLOG.md` (single implementation plan). Do **not** add a competing implementation plan.

**G00 rule:** Owner has approved the architectural choices and documentation work, **not** code. Close G00 only when these five documents, the updated backlog, schema/wire/mock and external-interface review evidence meet the gate and all required reviewers sign. Specific driver/native performance and legal relicensing remain gated by later qualification or legal review.
