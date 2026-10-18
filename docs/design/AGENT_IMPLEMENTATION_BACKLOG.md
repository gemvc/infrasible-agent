# Infrasible Monitoring Agent — Phased Implementation Backlog

> **Greenfield scope:** New clean-install daemon; no legacy consumers, migrations or compatibility gates. Canonical decision: [Product Requirements](AGENT_PRODUCT_REQUIREMENTS.md), section 2.

**Status:** FINAL OWNER-APPROVED PRODUCT/ENGINEERING ARCHITECTURE (DEC-01–11 + AD-01–03, 2026-10-18); G00 EVIDENCE / EXTERNAL SIGN-OFF PENDING; NOT AUTHORIZED FOR IMPLEMENTATION  
**Scope:** standalone open-source Linux Agent and the *minimum* external boundary contracts needed to test it  
**Repository:** https://github.com/gemvc/infrasible-agent  
**Baseline:** implementation `2f93988bf37243d7a7d57d1a4d4172bec1b8302c`; approved-doc source audit `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed` (2026-10-18)  
**Baseline product:** `agentVersion=0.1.0`, `schema_version=1.0`, `protocol 1.0`  
**Document owner:** Agent architecture/product owner (approval required); execution by assigned engineering and QA owners  
**Change policy:** This backlog is documentation only. It neither approves implementation nor authorizes changes to Desktop, Bridge, Persistence service, PostgreSQL, or production hosts.

> **How to use this file:** DEC-01–11 and the engineering architecture choices were approved by the owner on 2026-10-18 and documentation updates authorized. This is the single execution backlog. G00 stays BLOCKED pending cross-owner interface/QA evidence. Separate permission is still needed to implement runtime. Implement subsequent phases in dependency order using one small PR per deliverable or logically indivisible set. A phase is **done** only when its evidence and gate are accepted. “Implemented” without tests and failure evidence is **not done**. Entries labeled **PROPOSED** are not product commitments.

## 1. Product destination and non-negotiable architecture

The Agent runs **on each monitored Linux host**, commits one actual **1Hz raw frame** while collecting metric families at **independent 1s/5s/30s or startup/change intervals**, stores genuine metric observations/status/timestamps in a **single SQLite database with exactly three logical tables**, and performs UTC 1-minute then UTC 15-minute rollups. ACKed final 15-minute aggregates are locally retained 60 days from period end; **unACKed** finals survive past 60 days until valid ACK even if storage eventually degrades visibly. The Agent initiates outbound connections; there is no periodic reopening of SSH to collect live samples.

Two logically independent outbound channels exist:

- **Live:** 1Hz raw **frames containing only actually due/new measurements** (with optional clearly labeled old display cache) over an authenticated encrypted persistent connection to a **stateless Live Bridge**. The Bridge relays to subscribed Desktop Live UIs, does not persist history, and does not serve historical reads. Live delivery cannot block collection or durable storage.
- **Persistence:** *finalized 15-minute aggregates only* to a separate Persistence API. After a durable, idempotent PostgreSQL commit is certified by the API's ACK, the corresponding Agent row receives `persisted_at`. TCP delivery, an HTTP receipt, or a Live ACK does **not** count as durable persistence.

The historical UI obtains data exclusively through the central PostgreSQL-backed Historical API. The Agent never writes directly to PostgreSQL. An ordered array of **N** registered Monitoring endpoints defines priority by array index, with **independent** selection and recovery per channel. All outbound communication is Agent-initiated; authenticated and authorized mTLS transport is required at the approved protocol boundary.

### Locked invariants (must be tested, not weakened)

| ID | Invariant |
|---|---|
| INV-01 | Exactly `server_info`, `raw_samples`, `aggregates` are the Agent's initial logical SQLite tables; no JSONL, Redis, queue DB, broker, or fourth application table without approval. |
| INV-02 | Raw samples carry an explicit `saved` flag; it becomes true **only** in the same atomic transaction that durably commits the corresponding minute aggregate. Only saved raw samples are cleanup-eligible. |
| INV-03 | A 15-minute result is derived from finalized minute records transactionally and idempotently; minute source rows cannot be cleaned before durable 15-minute creation. Missing data is explicitly represented, not invented. |
| INV-04 | `persisted_at` applies to final 15-minute records only and is set **only after an authenticated durable database ACK**. ACK loss or retry cannot produce duplicate logical central records. |
| INV-05 | Live and Persistence have independent state, retry, resource budgets, server selection, and backpressure. Failure of one never blocks the other, sampling, or SQLite writes. |
| INV-06 | The Live Bridge has no monitoring history store or historical query responsibility; historical UI reads only via Historical API. |
| INV-07 | UTC windows use `[start,end)`, 60s/900s, and preserve **per-metric cadence, scheduled opportunities, actual valid/error/missing counts, original measured times, freshness, measured extrema/peaks and quality** through rollups; frame-level 60/900 counts do not replace slow-metric 12/2 counts per minute. |
| INV-08 | Endpoint priority is the **order of the N-element array**; at most one active endpoint **per channel**; no two-server limit or parallel duplicate active delivery. |
| INV-09 | Authenticated TLS is **not** sufficient by itself: the target must separately authorize this Agent and the requested channel. Invalid identity/cert/permissions fail closed. |
| INV-10 | Preserve audited collector correctness, but **the new public Agent is daemon-only and removes v1 version/snapshot/stream --json CLI**. No legacy Desktop consumers or migration exist; proprietary SSH/credentials remain out of scope. |
| INV-11 | **Owner-approved AD-01:** one nominal 1Hz raw frame; CPU/Network/Memory/Swap/Disk-I/O measurements 1s, Load 5s, Filesystem capacity 30s, host metadata startup/change. `not_due` is never a missing due observation and cached old values never create fresh samples. |
| INV-12 | **Owner-approved AD-02:** `valid` zero, `missing`, `error`, `counter_reset`, `unsupported`, `not_due`, and derived `stale` have distinct semantics; originals and age/TTL remain in Live and per-metric data-quality/coverage in 1m/15m. |
| INV-13 | **Owner-approved AD-03:** lean single non-root Go daemon; built-in bounded collectors and workers, health from first vertical slice; no generic plugins/AI/embedded UI, inbound public REST API, separate queue store or automatic host mutation. |

### Central retention boundary (NOT to be implemented here)

The Agent transmits 15-minute final records; the central service's planned retention is 15-minute rows for 30 days, hourly summaries for one year, then daily summaries for long-term retention. The central service must define late-arrival, deduplication, recomputation, and ACK semantics. The Agent phase builds only mocks/contract tests of these boundaries, **not** production Bridge, APIs, or PostgreSQL schema.

## 2. Grounded AS-IS baseline

Files inspected on the pinned baseline:

| Real file | Verified AS-IS | Change class |
|---|---|---|
| `main.go` | `cpuSample`, `cpuUsage`, `memoryFrom`, `loadFrom`, `interfacesFrom`, `filesystems`, `snapshot`; CLI `snapshot/stream` writes NDJSON to stdout | KEEP collector behavior; REFACTOR into testable modules |
| `version.go` | v0.1.0; `version --json`; schema/protocol v1.0; capabilities `snapshot`, `stream` | REPLACE public CLI contract with daemon version/Hello contract; reuse only suitable metadata logic |
| `main_test.go`; `version_test.go` | Basic parser/contract tests; no persistence, crash, failover or mTLS tests | KEEP and ADD cases |
| `go.mod` | Go 1.23, no third-party dependencies | REEVALUATE only justified SQLite/transport dependencies |
| `docs/PROTOCOL.md` | Current stdout JSON/SSH-client v1 contract; counter rates calculated by client | KEEP as AS-IS record; introduce separately versioned network contract |
| `docs/INSTALLATION.md`; `README.md` | Baseline installation/docs; some release/supported-platform statements need reconciliation | UPDATE only once decisions land |
| `scripts/package-linux.sh` | CGO-free linux amd64, arm64, armv7, armv6 builds; tarballs, select deb/rpm, SHA256SUMS and legacy raw sidecars | KEEP supported outputs; HARDEN and QUALIFY |
| `.github/workflows/ci.yml`; `release.yml` | vet, race tests, package/build/checksum; native amd64 CLI smoke; tag-driven release | EXTEND platform/security/runtime gates |
| `LICENSE` | MIT; provenance/copyright text requires owner review before Apache-2.0 transition | LEGAL REVIEW |

GitHub **v0.1.0 release and its CI/release workflows succeeded** on 2026-10-17; this establishes baseline packaging, **not** functionality of the new architecture. Exact source: https://github.com/gemvc/infrasible-agent/tree/2f93988bf37243d7a7d57d1a4d4172bec1b8302c . Existing code has no SQLite, registration, mTLS network client, daemon, dual channels, or active–standby logic. It is not a production-ready unattended monitoring service.

Source-inspection findings to reproduce in tests rather than assume fixed:
- `cpuUsage` sums all CPU ticks including `guest`/`guest_nice`, which are already included in Linux user/nice time; verify possible double counting and correct accounting with fixtures.
- `filesystems` and `interfacesFrom` omit some read/parse/scanner failure signals; incomplete inventories may appear without explicit errors.
- `stream` currently loops until process termination with no explicit graceful cancellation/flush lifecycle.
- New aggregation needs counter **rates**, reset/wrap handling and missing/partial-sample semantics, not merely the current cumulative bytes.
- Historical GitHub Release v0.1.0 exists, but is not a supported deployment or migration requirement for the new daemon.
- Cross-compilation for ARM is not native runtime qualification; checksum alone is not artifact publisher authentication.

## 3. Approved decisions and G00 evidence dependencies

**Approved by product owner in conversation 2026-10-18:** DEC-01–DEC-11 and reviewed engineering resolutions. Approval includes this docs-only repository update; it is **not** code, service, license-change, production or release authorization. The final wire details are subject to external interface-owner review and tests.

| ID | Owner-approved outcome | Gate evidence still required |
|---|---|---|
| DEC-01 | WSS persistent Live mTLS; HTTPS keep-alive Persistence mTLS; independent channels | Approve external wire/Hello/ACK fixtures |
| DEC-02 | SSH initial install/enrollment, stable agent_id in SQLite, Agent-originated mTLS | Issuer, token/CSR, rotation/revocation contract |
| DEC-03 | UTC 120-second minute grace, then immutable | Clock-step, late-data and restart tests |
| DEC-04 | ACKed final 900s retention 60 days; unACKed protected until ACK | ENOSPC/61d/replay/health tests |
| DEC-05 | Persistence HA only verified shared durable/dedup domain, Live independent | Domain proof and split-domain negative tests |
| DEC-06 | CGO-free Pure Go SQLite WAL + synchronous=FULL, controlled writer | Driver benchmark/native ARM and crash durability |
| DEC-07 | Daemon-only, **remove version/snapshot/stream --json** from new binary | Greenfield fresh install; zero legacy consumer migration; core SSH unchanged |
| DEC-08 | Peak RSS <=128MB; mean CPU <=5% of one core; jitter P99 <=500ms | Native reproducible test; disk and target >=2 final/s replay |
| DEC-09 | Apache-2.0 intended after rights audit, NOTICE and signed release | Current MIT remains; legal/provenance/signature verification |
| DEC-10 | Controlled corruption recovery, same-host ID retained, cloned host gets new ID | Restore/clone/registrar identity tests |
| DEC-11 | server_info = single host/Agent; ordered N endpoints in one versioned JSON config | Atomic reload/N=1,3,10/ACL tests |

**Five canonical P00 documents now authored (design, not executable proof):**
- [Product requirements and approved DEC outcomes](AGENT_PRODUCT_REQUIREMENTS.md)
- [Architecture, lifecycle and failure policy](AGENT_ARCHITECTURE.md)
- [Exactly three SQLite tables, DDL, transactions, retention](AGENT_SQLITE_SCHEMA.md)
- [Config, mTLS, WSS/HTTPS, ACK, failover, threat model](AGENT_PROTOCOL_AND_SECURITY.md)
- [P00-AC-01–20 and executable AC-001–055](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md)

**Approved engineering design defaults:** non-root systemd, one monotonic 1Hz frame scheduler with genuine due measurements (1s CPU/Network/Memory/Swap/Disk-I/O, 5s Load, 30s filesystem space, startup/change metadata), first-class per-metric quality/freshness, one SQLite writer, two bounded independent network workers; initial Live queue 2 events dropping oldest with counted quality; JSON config with atomic reload; locally generated key with time-limited one-use enrollment token; initial backoff 1–60s and healthy failback dwell >=90s as tunable values; v2 daemon wire, immutable retry payload+SHA256, 60-day ACKed cleanup and 61-day unACKed preservation. Exact drivers, hardware numbers and external services are evidence-gated, not magically implemented by these documents.

**Final owner-approved metric refinement AD-01–03 (2026-10-18):** one 1Hz raw frame, metric schedules 1s CPU/Network/Memory/Swap/Disk I/O, 5s Load, 30s filesystem capacity, metadata on startup/change; `valid/missing/error/counter_reset/not_due/stale/unsupported` distinct, original timestamps and age/TTL on Live, per-metric coverage/peaks retained through 1m/15m. One small non-root Go daemon with bounded modules and health from first vertical slice. This is **approved design**, not implementation or G00 test execution.

**Remaining G00 external agreements:** Bridge ingress/wire/subscriber behavior; Persistence verified durable PostgreSQL ACK, same-domain dedup including >30-day late arrivals and hourly/daily reaggregation; PKI issuer/revoke/clone; legal license review; QA/Native benchmark rigs. These are **evidence dependencies**, not reopened decisions. **G00 remains BLOCKED** until P00-AC-01–20 and cross-owner sign-off, with implementation authorization separate.

## 4. Work management rules

**Ticket workflow:** `BACKLOG → READY → IN_PROGRESS → REVIEW → VERIFIED → DONE`, or `BLOCKED` with a linked decision.  
**Priority:** P0 correctness, security, durability and gate blockers; P1 lifecycle, new-protocol integration and observability; P2 optional improvements explicitly excluded from the release path.  
**Per-ticket Definition of Ready:** approved source-of-truth/contract; dependent IDs completed; real touched paths identified; negative tests specified; measurable result and rollback plan known.  
**Per-ticket Definition of Done:** implementation/tests/updated docs in one reviewed PR, `go test ./...`, `go vet ./...`, `go test -race ./...` (or documented constrained-platform equivalent), relevant integration/failure tests, passing CI, no unjustified schema/service addition, and evidence linked in ticket. Local green does not override CI red.  
**Evidence:** PR SHA and test command/run URL, fixtures used, test logs or metrics, before/after new-contract fixtures, security review where applicable. Manual tests require platform, image, steps, expected and observed output.  
**Parallelization:** P01 can proceed after applicable P00 approvals; P02/P03 contracts must precede changes to the data pipeline; P06 can overlap P03–P05 after security/interface approval; P07 and P08 can overlap only after P06 contracts; P09 follows both. P10 preparations may start earlier but must not release prior to P11.

### Phase dependency map

```text
P00 product + architecture + boundary approvals
  ├─> P01 collector correctness & daemon foundation
  ├─> P02 fresh SQLite schema, identity and bootstrap
  │     └─> P03 1-second sampling & durable recording
  │            └─> P04 minute rollup
  │                   └─> P05 final 15-minute rollup & retention
  └─> P06 secure configuration, registration and mTLS
           ├─> P07 Live channel (also P03)
           └─> P08 Persistence/ACK/replay (also P05)
                  └─> P09 N-endpoint active–standby (also P07)
P01–P09 ──> P10 lifecycle, packaging, security, clean install
P00–P10 ──> P11 end-to-end qualification & release sign-off
```

## P00 — Architecture, contracts and acceptance baseline (P0)

**Outcome:** owner-approved final architecture, including multi-rate sampling, truthful age/quality and lean vertical delivery, translated into implementation-ready agreements **without runtime changes**. **Prerequisites:** pinned repository baseline.

| Ticket | Deliverable / precise scope | Verification and acceptance |
|---|---|---|
| AG-0001 | Maintain `AGENT_PRODUCT_REQUIREMENTS.md` with owner DEC-01–11, AD-01–03, 1Hz frames/1s-5s-30s metric cadences, freshness/status vocabulary, non-goals, SLOs and locked invariants. | **Test:** every locked rule and per-metric due/quality case maps 1:1 with no contradiction; owner-approved documentation and cross-owner evidence distinguished. |
| AG-0002 | Maintain `AGENT_ARCHITECTURE.md` with one scheduler, due-metric selection and bounded collectors, optional age-labeled Live cache, quality-first health, independent network channels, UTC rollup and fault states. | **Test:** deterministic walkthrough of 1s/5s/30s due, stale/zero/missing, crash/clock/network/overflow and early vertical slice; no implicit decision. |
| AG-0003 | Maintain `AGENT_SQLITE_SCHEMA.md` with three-table DDL, frame-level versus per-metric expected-opportunity JSON, stable real measurement timestamps, 1m/15m weighted sufficient statistics, WAL/FULL, retention and fresh-schema bootstrap. | **Test:** DDL and mock review validates 40/60 CPU, 8/12 Load, 2/2 capacity, no stale inflation, transaction/ACK invariants; data owner signs off. |
| AG-0004 | Maintain `AGENT_PROTOCOL_AND_SECURITY.md` with live frame measured-at/age/quality/metadata and final metric-specific coverage, in addition to N-endpoint config, mTLS, authorized durable ACK and failover. | **Test:** 1Hz fresh frames distinguish old Live display and not_due; central 900s mock preserves exact metric-specific history/ACK; security and external owners approve. |
| AG-0005 | Maintain `AGENT_ACCEPTANCE_AND_TEST_MATRIX.md` as canonical P00-AC-01–20/AC-001–055, with independent 1s/5s/30s, status/freshness/early-health negative-path fixtures; keep this as single phased backlog. | **Test:** all 57 tickets, 55 executable AC and 20 design AC trace without duplicate implementation plan; owner approves design, G00 still evidence-gated. |

**Gate G00:** five canonical docs, updated backlog and P00-AC-01–20 evidence explicitly reviewed by product/data/security/QA/external owners, baseline pinned and DEC outcomes recorded. **Current status: BLOCKED pending external sign-off/evidence; no implementation authorization.**

## P01 — Baseline correctness, CLI and testability (P0)

**Outcome:** preserve audited collector semantics, but create **daemon-only v2** (legacy v0.1.0 is reference-only, not a migration artifact). **Prereq:** G00 and DEC-07.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0101 | Extract Linux parsers/collectors from `main.go` with injectable `/proc` fixtures and versioned daemon metrics/quality structs. | Golden collector fixtures pass; new daemon has no requirement to emit legacy CLI JSON. |
| AG-0102 | Correct Linux CPU tick delta accounting (guest duplication, zero/negative deltas, hotplug/reset) and confirm IOWait semantics. | Table-driven synthetic `/proc/stat` tests cover guest, wrap/reset and all-idle; output stays in valid 0–100% ranges, invalid epochs are flagged rather than fabricated. |
| AG-0103 | Surface partial filesystem/network errors, mount/interface parsing problems, permission loss and counter semantics without treating zeros as valid missing readings. | Fixtures for corrupt `/proc/net/dev`, unreadable mounts, escaped mount paths and permission error; observable quality/error fields with versioned compatibility. |
| AG-0104 | Daemon-only signal cancellation, controlled writer drain/rollback and graceful shutdown; no public stream command. | SIGTERM/SIGINT cannot strand partial SQLite transaction, socket, goroutine or service process; no per-second SSH. |
| AG-0105 | Define clean-install daemon protocol-major 2 version/health/Hello and document new package lifecycle; old v0.1.0 serves only as a reference. | Native clean-install daemon smoke and new protocol contract fixtures; no legacy migration tests. |

**Gate G01:** AC-001–004 pass for greenfield daemon; no legacy consumer migration gate exists.

## P02 — SQLite foundation, identity and clean bootstrap (P0)

**Outcome:** one crash-safe database; no transport or aggregation yet. **Prereq:** G00, DEC-02/04/06/10/11.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0201 | Benchmark and select maintained SQLite Go driver against CGO-free builds, lock handling and all target architectures; record driver and durability ADR. | Build all 4 targets; run native smoke for accessible hosts, identify unqualified targets; microbench WAL write/commit, FULL vs alternatives; results attached. |
| AG-0202 | Implement one DB open/close lifecycle, a controlled writer, busy handling, WAL, approved sync level and integrity/readiness reporting. | Competing readers + one writer under stress cause no deadlock or corruption; injected BUSY returns bounded retry/health; abrupt termination is recoverable. |
| AG-0203 | Create only `server_info`, `raw_samples`, `aggregates`; define keys, indexes, explicit `saved`, interval 60/900, completeness/lineage, checksum/record ID, `persisted_at` final-only. | Schema introspection verifies exactly three application tables, indexes/constraints enforced and invalid resolution/state rejected; no fourth queue table. |
| AG-0204 | Implement atomic first-install schema bootstrap, `user_version`, schema validation and fail-closed handling for an unknown version; no pre-launch migrations. | Empty DB, interrupted bootstrap, unexpected schema, restart and integrity tests pass. |
| AG-0205 | Persist immutable `agent_id` and host identity/metadata, config association and restore/reinstall policy; ensure cloned machines cannot silently duplicate identity. | Restart preserves identity, clone/re-enroll test rejects collision, wrong-host restore requires explicit recovery approval, no secret leaked via logs. |

**Gate G02:** AC-005–008 pass; DB invariants validated under forced crash, filesystem lock and interrupted first-time bootstrap.

## P03 — Continuous 1Hz frames and independent metric sampling (P0)

**Outcome:** one reliable 1Hz raw frame stream with independent 1s/5s/30s metric schedules, original measured times, truthful quality/freshness and first-slice health; no Desktop SSH polling. **Prereq:** G01, G02, DEC-03, AD-01–03.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0301 | Implement one cancellable monotonic **1Hz frame scheduler** with versioned due-profile/phase for 1s/5s/30s collectors, real frame and measurement times, stable IDs, bounded execution and overrun policy. | Deterministic one-hour clock yields 3600 frames, Load 720 due, capacity 120 due; no fake measurements/goroutine runaway, missed frames explicit. |
| AG-0302 | Build small Go modules for CPU/Network/Memory/Swap/Disk-I/O 1s, Load 5s, filesystem space 30s, metadata startup/change; each has original measured_at, unit, entity, due/error/reset/unsupported and freshness TTL semantics. | Linux fixtures cover independent due schedules, measured zero versus error/null, counter reset, mount churn, old cache, 2x cadence+jitter TTL and no general plugin engine. |
| AG-0303 | Persist one SQLite row **per actual one-second frame**, recording *only* genuine due readings or due failures, profile version and typed quality; keep old-value Live display cache in bounded RAM only. | Offline ≥30 min plus 5s/30s not_due frames: no cloned old readings in raw, no stale/zero confusion, no unbounded buffers and explicit gaps. |
| AG-0304 | Safe daemon startup/shutdown, structured health from first slice (per-metric last-good/due/error/age, Storage, channels), deterministic fake-clock injection; no legacy public stream. | 1h soak/restart preserves actual measurement times and profile, reports stale when TTL elapses, no duplicate frames or misleading all-healthy flag. |

**APPROVED DEC-08 acceptance target:** nominal 1s cadence; schedule-start jitter P99 <=500ms; peak RSS <=128MB; mean CPU <=5% of one core on an explicitly documented reference workload. Benchmark calibration/native-platform qualification remains REQUIRED. Missing samples are visible as `coverage` degradation, never silently filled.

**Gate G03:** AC-009–012 pass; 1-hour workload/partial-data evidence; no SSH reconnection per tick.

## P04 — Transactional UTC 1-minute aggregation (P0)

**Outcome:** trustworthy minute summaries and safe raw cleanup. **Prereq:** G03, DEC-03.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0401 | Define aligned `[UTC_minute, UTC_minute+60s)` source queries; close-window/grace and late-sample behavior. | Boundary tests at xx:xx:00.000 and xx:xx:59.999; DST/timezone and NTP clock-step tests; exact source IDs stable. |
| AG-0402 | Compute **per-metric scheduled/attempted/valid/missing** denominators (not global frame count), measured_at, bounded weighted-gauge statistics, actual extrema/peak, freshness/TTL and per-entity quality; no stale cache inflation. | Fixtures show CPU 40/60 spike, Load 8/12 and Capacity 2/2, with not_due frames excluded from misses, and no unseen capacity spike claim. |
| AG-0403 | In **one SQLite transaction** upsert/finalize minute row and set `saved=true` only on its exact source raw IDs; ensure idempotency and retry after crash. | Failpoint **before, during and after commit**; restart yields one minute aggregate and all-or-none saved flags. No sample is marked saved without durable minute. |
| AG-0404 | Delete **only** already-saved raw rows in bounded batches; permit cleanup to resume across shutdown and busy errors. | Verify unsaved row survives expired staging age, crash during DELETE, disk pressure and concurrent reader; data loss count = 0 for unsaved records. |

**Gate G04:** AC-013–017 pass, including atomic crash-failpoint tests and spike/coverage golden files.

## P05 — Transactional 15-minute finalization, queue and 60-day retention (P0)

**Outcome:** self-contained finalized historical rows, safe local replay queue. **Prereq:** G04, DEC-03/04.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0501 | Build immutable 900s aggregate of **metric-specific** 1s/5s/30s opportunity counts, valid durations, actual extrema/peak timestamps, errors/age/anomalies and per-entity coverage. | Expected healthy quarter due opportunities 900/180/30, actual measured CPU peak survives; stale cache never multiplies into history. |
| AG-0502 | Transactionally create a stable **immutable** 900s record, with deterministic record ID/lineage, and clean minute rows **only after** durable finalization. | Crash at each transaction boundary; retry produces one identical 15m record; no minute row disappears before a committed final record. |
| AG-0503 | Define indexes and pending queue view logically as `resolution=900 AND persisted_at IS NULL`, oldest first. **No extra queue DB/table.** | Insert out-of-order, partially complete and many pending periods; indexed query returns stable oldest-first IDs on restart. |
| AG-0504 | Enforce approved 60-day policy for finalized rows, distinguished by ACK state, observability for old pending rows, batch maintenance, WAL checkpoint and capacity planning. | Simulate ≥61 days in test clock; ACKed expiry follows policy; unACKed records never disappear silently; disk pressure raises explicit health signal. |
| AG-0505 | Produce documented size estimates using measured JSON/entity sizes, samples, filesystem/interface cardinality, SQLite/WAL overhead and replay capacity. | Formula and 60-day generated fixture reviewed; actual measured DB and WAL stay within approved budget for reference host. |

**Capacity sanity check, not a quota:** 60 days × 24 h × 4 finalized records/hour = **5,760** final records per Agent. At an illustrative **16 KiB per final record**, payload-only volume ≈ **90 MiB**; actual SQLite rows/indexes/WAL, extra retained unACKed records and cardinality must be measured. Raw/one-minute staging is *not* 60-day history.

**Gate G05:** AC-018–022 pass; 61-day simulated retention with offline pending-data protection and durable stage-by-stage crash tests.

## P06 — Registered endpoints, identity and mTLS (P0)

**Outcome:** secure bootstrap and outbound connection framework; still no central service implementation. **Prereq:** G00 + DEC-01/02/05/11; can be developed alongside P03–P05.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0601 | Versioned **single** configuration file with ordered N endpoint entries, independently declared Live/Persistence URLs/capabilities, trust anchors, server name binding and no duplicate priority field. | Parser rejects malformed version, duplicate endpoint identity, unsafe schemes, missing authorities and unknown critical fields; N=1 and N=3 valid. |
| AG-0602 | Establish reviewed initial registration, stable `agent_id` ↔ cert binding, endpoint/channel authorization, trust store, protected key material and safe re-enrollment. | Wrong agent, unknown endpoint, missing channel grant, invalid cert chain and stolen/expired cert are rejected; logs redact credentials and tokens. |
| AG-0603 | Implement outbound mTLS connection factory (bounded timeouts, context cancellation, keepalive/heartbeat and resource limits), with certificate rotation and revocation handling. | Tests for bad SNI, hostname mismatch, expired/rotated/revoked cert, MITM/self-signed CA not trusted and no plaintext fallback. |
| AG-0604 | Versioned Hello/capability negotiation, separately defined authn/authz failures, documented request/message size and rate limits; fixture-based contract compatibility. | Unsupported major version fails explicitly; permitted minor versions negotiate; unknown/unauthorized agent cannot open an operational channel. |

**Gate G06:** AC-023–027 pass; approved security threat model and negative authentication/authorization suite.

## P07 — Nonblocking Live channel to stateless Bridge (P0)

**Outcome:** live samples over one selected persistent Agent-initiated connection; no history dependence. **Prereq:** G03, G06, DEC-01.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0701 | Define Live 1Hz **frame** envelope with only fresh due 1s/5s/30s observations and per-metric original measured_at/status; optional old display cache must carry source time, TTL/age and new_observation=false. | Golden mock fixtures reject repeated old metrics as fresh or not_due as missing; no history storage or mismatched identity. |
| AG-0702 | Add bounded non-durable outbound live buffer, explicit drop/backpressure metrics and independent goroutine lifecycle. | Slow/blocked mock Bridge cannot delay collector commit or minute/final rollups; bounded memory; dropped live events counted. |
| AG-0703 | Implement reconnect and heartbeat with timeout/jitter, no per-second SSH or connection open/close; backfill semantics explicitly **not** historical replay. | Force 10 disconnections and recovery; no runaway goroutines/socket count; UI gets new live samples after reconnection; no fabricated historical stream. |
| AG-0704 | Provide mock Bridge contract tests that relay to a live subscriber and contain **no DB/history reads/writes**. | Test double assertions: no historical query or storage interface exercised; offline Persistence still permits live relay. |

**Gate G07:** AC-028–031 pass; live soak with blocked bridge demonstrates collection/storage independence.

## P08 — Durable Persistence channel, ACK and replay (P0)

**Outcome:** final records reach *only* the Persistence API and are acknowledged by proof of durable, idempotent central commit. **Prereq:** G05, G06, DEC-01/03/04/05.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0801 | Define immutable final-only 900s envelope with stable ID/hash and per-metric cadence, original measured peak/last, quality/stale, independent coverage and profile version alongside frame coverage. | Mock rejects raw/60s, wrong agent/hash/domain and a history payload that turns not_due/cache into valid samples. |
| AG-0802 | Implement oldest-first pending replay with bounded in-flight work, authenticated transport, retryable/permanent error classification, exponential backoff + jitter. | Simulate outage, throttling and restart; pending order preserved, no hot retry loop, stream/collection unaffected. |
| AG-0803 | Verify a **durable idempotent commit ACK** that binds agent ID, record ID, interval, payload hash/schema and approved persistence domain; atomically set local `persisted_at` **only after** valid ACK. | Fake HTTP 200 lacking durable ACK, forged ACK, wrong hash/domain/ID or transient error never changes `persisted_at`. |
| AG-0804 | Handle commit-success/ACK-lost split-brain retry: same stable record ID and payload; central mock upserts/deduplicates without duplicate logical row. | Kill network after central commit before ACK; restart/resubmit N times → exactly one logical central row and one local persisted timestamp after valid ACK. |
| AG-0805 | Independently handle overdue/rejected pending records, 60+ day interruption, storage exhaustion and operator-visible status. | Offline 1/30/61 days simulation produces explicit backlog/age/space evidence and **no silent discard**; recovery throughput benchmark documented. |

**Gate G08:** AC-032–038 pass; end-to-end mock central durable commit and deterministic ACK-loss replay; no central API implementation added to this repo.

## P09 — Ordered N-endpoint Active–Standby and failback (P0)

**Outcome:** predictable channel-independent HA with finite resources. **Prereq:** G07, G08, DEC-05.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0901 | Implement per-channel deterministic eligibility and priority selection from config index, with `DISCONNECTED → AUTHENTICATING → ACTIVE → BACKOFF/FAILOVER`, distinct channel state. | N=1/3/10 fixture, independent capability/authz failures; exactly one active server per channel, smallest eligible index preferred. |
| AG-0902 | Per-channel health probes, deadlines, bounded backoff+jitter, anti-flap minimum dwell, higher-priority failback hysteresis and per-host connection budget. | Synthetic intermittent faults and recovered primary do not flap; failover and failback meet approved bound; no connection explosion as N grows. |
| AG-0903 | Support Live and Persistence selecting **different** currently eligible endpoints without cross-blocking. | Live primary down + Persistence primary up, reverse and simultaneous outage; live stream and replay progress match their own channel states only. |
| AG-0904 | Bind durable ACK/dedup domain to authorized central domain; prevent treating Server A's ACK as evidence Server B contains identical data. | Two mock central stores (shared vs independent) prove safe selection and explicit blocked/alerted unsafe failover; replay remains idempotent within domain. |
| AG-0905 | Config reload and endpoint removal/revocation, deterministic re-evaluation; ensure old unauthorized peers cannot retain access. | Delete/reorder/disable endpoint under load; safe drain/cancel, no credentials leak, no unauthorized persisted ACK. |

**Gate G09:** AC-039–044 pass with recorded N=3 failover/failback traces and domain-isolation negative tests.

## P10 — Unattended operations, security and clean-install packaging (P0/P1)

**Outcome:** manageable Linux service without changing Desktop core SSH. **Prereq:** G01–G09, DEC-07/09/10.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-1001 | Daemon-only entrypoint and safe install/uninstall/systemd service, journald health, liveness and deterministic drain/restart. | systemd restart/stop/kill leaves no half-committed sample/orphan process/privilege escalation/public listener. |
| AG-1002 | Least-privilege runtime user, controlled DB/config/cert paths, strict key permissions, refusal of unsafe config/cert ownership, log redaction and resource limits. | Non-root smoke; permission/owner attacks rejected; secrets never printed, world-writable key/config rejected. |
| AG-1003 | Specify and test fresh daemon installation, repeatable provisioning, secure startup, clean removal and same-host disaster recovery; defer post-release upgrade migration. | Native clean-install/removal tests pass; identity/recovery safe; core SSH unchanged; no legacy package coexistence. |
| AG-1004 | Sign/check release provenance and artifact digests; update tar/deb/rpm for approved service layout, new deps and correct license/copyright/NOTICE. | Independently verify trusted signature and SHA, package metadata, install/upgrade/uninstall on native supported OS, no leftover service state after safe uninstall. |
| AG-1005 | Expand CI to protocol, failpoint, race, security, packaging, signed release and **native** ARMv6/ARMv7/ARM64 qualification (lab or controlled emulation clearly labeled). | Cross-compiles all targets; native reference runs recorded separately; unsupported targets explicitly excluded/marked experimental before publish. |
| AG-1006 | Complete docs: clean install, registration, recovery, trust/rotation, protocol versions, resource/sizing, runbook and licensing. | Fresh-install documentation drill from bare Linux VM to observed live + durable historical mock ACK, with no missing secret steps. |

**Gate G10:** AC-045–051 pass; real deployment rehearsal on disposable hosts only, security/license approvals attached. Production deployment is a separate authorization.

## P11 — Full acceptance, resilience, performance and release (P0)

**Outcome:** evidence-backed release candidate and closed Agent scope. **Prereq:** all prior gates.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-1101 | Run full deterministic integrity/fault matrix, including 1s/5s/30s due failures, cached stale/true zero, crash/clock/profile changes, SQLite, Live, auth, ACK, 60d retention. | All INV-01–13 hold; gaps/errors and independently stale metrics observable; no fabricated success or removal of unsaved/pending. |
| AG-1102 | Run N=3 Live/Persistence E2E with independent outages, per-metric 1s/5s/30s Live frames, final-only 900s quality/coverage, hash/replay, shared/split domains and 60+ day recovery. | Bridge never stores history; 900s summary retains exact per-metric count/quality; no stale inflation or duplicate durable rows. |
| AG-1103 | Benchmark one-hour native 1Hz frames, actual 1s/5s/30s collection, health/quality cost, high-cardinality mount/interface and disk I/O, SQLite/WAL/retention and 60-day replay. | CPU <=5% one core mean, peak RSS <=128MB, frame P99 jitter <=500ms, counts 3600/720/120 due on uninterrupted hour; otherwise release BLOCKED. |
| AG-1104 | Verify platform matrix for Linux amd64, arm64, armv7, armv6: build, native smoke, service, network auth, SQLite integrity and artifact authenticity. | Every advertised production platform passes native qualification; unqualified platforms are not claimed as production-supported. |
| AG-1105 | Review security (spoof/replay/token/cert/ACL/key exposure), docs, clean install/disaster recovery, license provenance, API contract owners and all linked evidence; freeze Agent scope. | All P0/P1 blockers closed, owner checklist signed; versioned tag/release is authorized separately, no unreviewed runtime/API behavior. |

**Gate G11 — Release candidate acceptance:** all signed INV + AC criteria, mandatory CI and native matrix, approved quantitative SLOs, release audit, owner sign-off; **only then** tag/release under separately approved execution authority.

## 5. Stable acceptance IDs — canonical test matrix

The **authoritative Given/When/Then acceptance text** is [AGENT_ACCEPTANCE_AND_TEST_MATRIX.md](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md). This backlog references its stable IDs, so duplicate changing test definitions are deliberately removed here. **AC-003, AC-004, AC-047** now reflect approved DEC-07 daemon-only; P00-AC-01–20 are the design gate.

| IDs | Phase | Coverage |
|---|---|---|
| P00-AC-01–20 | P00 | Design/docs/external agreement sign-off |
| AC-001–004 | P01 | Collector/daemon/clean installation |
| AC-005–008 | P02 | SQLite/identity/recovery |
| AC-009–012 | P03 | 1Hz raw frames; real per-metric 1s/5s/30s cadence, due failures, fresh/stale and early health |
| AC-013–017 | P04 | 1m per-metric opportunity denominators, quality and saved atomicity |
| AC-018–022 | P05 | 15m metric-specific weighted rollup, 60d/61d retention |
| AC-023–027 | P06 | mTLS/config/authorization |
| AC-028–031 | P07 | stateless live bridge |
| AC-032–038 | P08 | durable ACK/replay |
| AC-039–044 | P09 | N endpoint HA |
| AC-045–051 | P10 | systemd/signed native packages/clean installation |
| AC-052–055 | P11 | E2E/fault/performance/release |

Negative fixture inventory and explicit G00–G11 evidence/sign-off are maintained only in the canonical acceptance matrix.

## 6. Boundary contracts the Agent needs; external services remain out of scope

The following are **contract tests against mocks** in this repo. They are not permission to implement another service.

| Contract | Minimum external guarantee |
|---|---|
| Live Bridge | Authenticated + authorized inbound 1Hz Agent frame with original 1s/5s/30s due measured_at, optional age-labeled old display cache, subscribed UI forwarding, no history or historical query; close/error/heartbeat semantics |
| Persistence API | Authorized `agent_id`, **900s final-only** with frame and per-metric expected/valid/error/quality fields, stable-ID idempotent durable PostgreSQL commit, verifiable ID/hash/domain ACK and typed retryability |
| Historical API | UI historical reads reflect durable central records; never bridge/Agent SQLite; late-arrival correction and central 30-day/1-year/long-term policies documented by their owner |
| Monitoring registration/PKI | Trusted certificate authority, server identity, Agent enrollment/rotation/revocation, allowed endpoint/channel mappings; controlled restoration |
| Desktop/SSH integration | Greenfield only: no existing consumers or old-agent coexistence. New daemon removes public v1 CLI; new Desktop integration targets the new contract; core SSH/vault unchanged. |

**Canonical v2 wire samples and durable ACK** are in [AGENT_PROTOCOL_AND_SECURITY.md](AGENT_PROTOCOL_AND_SECURITY.md). Do not duplicate illustrative JSON with conflicting hash and domain semantics in this implementation backlog. External endpoint owners must sign compatible fixtures.

## 7. CI, evidence and release-gate commands

Mandatory Go checks when code exists:

```bash
go fmt ./...                       # check git diff after formatting
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
bash scripts/package-linux.sh
(cd dist && sha256sum -c SHA256SUMS)
```

Additional stage commands/scripts to **add**, not assumed existing: schema constraint suite, deterministic fake-clock 61-day scenario, per-transaction killpoint harness, expired/revoked certificate suite, mock Bridge/Persistence compatibility suite, lost-ACK central dedup test, N=3 HA suite, long soak/high-cardinality benchmark, signed artifact verification, native ARM matrix. CI must distinguish required CI tests from manual or platform-lab acceptance; manual tests require recorded evidence.

Recommended evidence path convention (PROPOSED; avoid committing bulky test output): `docs/design/evidence/README.md` describing CI links, signed benchmark reports, runner images and known limitations. Avoid duplicate AS-IS specs: `README.md` and existing `docs/PROTOCOL.md` remain current-version operational docs; design documents define approved target architecture.

**Release candidate checklist (all required):**

- [ ] DEC-01–11 and **AD-01–03** approved, including 1Hz frame with independent metric due policies and first-class quality/freshness; P00-AC-01–20 and external G00 sign-offs recorded.
- [ ] G01–G10 and AC-001–055 verified with linked evidence.
- [ ] No unknown “partial success” for raw saved, minute final, 15m final or central durable ACK.
- [ ] All advertised platforms native-tested; 1s/5s/30s due cadence and metric freshness correctness, CPU, RSS, disk/WAL, jitter and catch-up thresholds measured and met.
- [ ] N=3 separate-channel failover/failback and ACK-domain test passes.
- [ ] No history through Bridge, no raw/minute to Persistence API, no PostgreSQL directly from Agent; no cached stale value reported as a newly measured metric.
- [ ] 60-day final history and 61-day pending/recovery policy proved without silent data loss.
- [ ] Negative mTLS/authz, replay, clone and cert rotation cases pass.
- [ ] Fresh-install daemon-v2 contract verified; no legacy CLI migration required; Desktop SSH remains untouched.
- [ ] Updated install/config/recovery docs, license/NOTICE and publisher-signature verification pass.
- [ ] Product, architecture, data, security, QA, and external-contract owners explicitly approve.
- [ ] No code/production mutation or release is inferred from approval of this backlog document alone.

## 8. Ticket issue template (copy per AG-xxxx)

```markdown
Title: AG-xxxx — <outcome>
Phase / Gate: Pxx / Gxx
Priority: P0 | P1
Status: BACKLOG | READY | IN_PROGRESS | BLOCKED | REVIEW | VERIFIED | DONE
Owner: <assigned person/team>
Depends on: <ticket IDs + approved DEC IDs>
Scope (in): <one independently reviewable change>
Out of scope: <explicit exclusions>
Touched source/doc paths: <actual paths verified before coding>
Design / protocol reference: <approved canonical doc + version>
Given / When / Then acceptance: <AC IDs plus precise expected values>
Automated tests: <commands, fixtures, failure injections>
Platform / manual evidence: <OS, arch, test rig, steps and artifacts>
Recovery / safe failure: <new-daemon crash recovery, identity continuity and fresh-install behavior; no legacy-client compatibility requirement>
Definition of Done: <CI link, tests, logs, review approvals, documentation>
```

## 9. Source-of-truth and change control

1. **Observed AS-IS:** pinned source commit and verified test/run evidence override stale README claims.
2. **Locked TO-BE:** owner-supplied Agent architecture guide takes priority over legacy implementation, unless explicitly re-approved.
3. **Approved TO-BE design:** `AGENT_PRODUCT_REQUIREMENTS.md` is canonical for product decisions; `AGENT_ARCHITECTURE.md` for design boundaries; `AGENT_SQLITE_SCHEMA.md` and `AGENT_PROTOCOL_AND_SECURITY.md` for their respective contracts; `AGENT_ACCEPTANCE_AND_TEST_MATRIX.md` for test requirements.
4. **This file** is the **single execution backlog and implementation sequence**, not an alternative system architecture. Do not create a redundant `AGENT_IMPLEMENTATION_PLAN.md`. Change IDs or gate criteria only with documented owner approval and linked impact analysis.
5. Require small PRs, review of real file/line impacts, explicit fresh-install/schema-version handling and evidence. This file is documentation; **no Agent code, repository release, service or production server changes are authorized by it**.

**Backlog completeness criterion:** Every mandatory product behavior has a ticket, acceptance ID, dependency, negative-path test and gate. Anything needed to ship but missing here is a scope blocker to add through reviewed backlog change—not an implicit “developer choice.”

## Desktop-controlled SSH configuration and storage maintenance — scope extension (2026-10-18)

**Status: BACKLOG / NOT IMPLEMENTED; G00 unchanged.** Operator installs Agent with any SSH-capable workflow and uses existing Desktop SSH/Ansible to edit the one versioned Agent config, set host alert thresholds and desired Agent resource limits, inspect local SQLite/disk occupancy, and request safe cleanup. This is **not** an inbound management API, new SSH core, second database, daemon alert engine or new runtime service. Desktop evaluates host CPU/RAM/filesystem alert thresholds; Linux systemd/cgroup enforces authorized Agent CPU/RAM ceilings via privileged SSH/Ansible actions. Storage pressure budget never authorizes discarding Pending data.

- **P00 / contract:** Review new MGMT-01–09 cases in the acceptance matrix, define supported config keys/ranges/revision model and native authorization scope; assign permanent AC IDs at gate review. External Desktop UX/threshold interpretation is a separate owner sign-off.
- **P02 / storage:** Implement storage inventory from existing tables and SQLite/file metadata; exact record counts, qualified estimated bytes, 60-day ACKed eligibility, Pending/unsaved protection; dry-run and bounded guarded cleanup/recovery tests. No fourth table or generic SQL admin endpoint.
- **P06/P10 / configuration and packaging:** Define versioned config validation, safe atomic replacement, effective revision and reload/restart diagnostics; add narrowly scoped **local** CLI for operator use over existing SSH. Add operator-owned systemd drop-in provisioning for CPUQuota/MemoryMax and confirm effective limits; Agent remains non-root and self-contained.
- **P11 / acceptance:** Prove MGMT-01–09 with filesystem race, unauthorized caller, malformed config, crash, stale alert quality, OOM/throttle, ENOSPC, WAL checkpoint, 1Hz nonblocking and Desktop-disconnected fixtures. Keep 128MB/5%-core DEC-08 performance targets separate from operator-selected hard ceilings.

No executable daemon/local CLI, Desktop UI, alert notifications, package updates or deployment is claimed by this documentation change.
