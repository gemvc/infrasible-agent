# Infrasible Monitoring Agent — Phased Implementation Backlog

**Status:** OWNER-APPROVED DEC-01–11 AND ENGINEERING DESIGN (2026-10-18); G00 EVIDENCE / EXTERNAL SIGN-OFF PENDING; NOT AUTHORIZED FOR IMPLEMENTATION  
**Scope:** standalone open-source Linux Agent and the *minimum* external boundary contracts needed to test it  
**Repository:** https://github.com/gemvc/infrasible-agent  
**Baseline:** implementation `2f93988bf37243d7a7d57d1a4d4172bec1b8302c`; approved-doc source audit `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed` (2026-10-18)  
**Baseline product:** `agentVersion=0.1.0`, `schema_version=1.0`, `protocol 1.0`  
**Document owner:** Agent architecture/product owner (approval required); execution by assigned engineering and QA owners  
**Change policy:** This backlog is documentation only. It neither approves implementation nor authorizes changes to Desktop, Bridge, Persistence service, PostgreSQL, or production hosts.

> **How to use this file:** DEC-01–11 and the engineering architecture choices were approved by the owner on 2026-10-18 and documentation updates authorized. This is the single execution backlog. G00 stays BLOCKED pending cross-owner interface/QA evidence. Separate permission is still needed to implement runtime. Implement subsequent phases in dependency order using one small PR per deliverable or logically indivisible set. A phase is **done** only when its evidence and gate are accepted. “Implemented” without tests and failure evidence is **not done**. Entries labeled **PROPOSED** are not product commitments.

## 1. Product destination and non-negotiable architecture

The Agent runs **on each monitored Linux host**, samples host metrics approximately once per second, stores monitoring state in a **single SQLite database with exactly three logical tables**, and performs UTC 1-minute then UTC 15-minute rollups. ACKed final 15-minute aggregates are locally retained 60 days from period end; **unACKed** finals survive past 60 days until valid ACK even if storage eventually degrades visibly. The Agent initiates outbound connections; there is no periodic reopening of SSH to collect live samples.

Two logically independent outbound channels exist:

- **Live:** 1-second raw samples over an authenticated, encrypted persistent connection to a **stateless Live Bridge**. The Bridge relays to subscribed Desktop Live UIs, does not persist history, and does not serve historical reads. Live delivery cannot block collection or durable storage.
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
| INV-07 | UTC period windows use `[start,end)` and exact 60s / 900s intervals; coverage, sample counts, max/min/last and peak/anomaly evidence survive rollups. |
| INV-08 | Endpoint priority is the **order of the N-element array**; at most one active endpoint **per channel**; no two-server limit or parallel duplicate active delivery. |
| INV-09 | Authenticated TLS is **not** sufficient by itself: the target must separately authorize this Agent and the requested channel. Invalid identity/cert/permissions fail closed. |
| INV-10 | Preserve audited collector correctness, but **the new public Agent is daemon-only and removes v1 version/snapshot/stream --json CLI**. Block silent incompatible replacement of legacy Desktop consumers; proprietary SSH/credentials remain out of scope. |

### Central retention boundary (NOT to be implemented here)

The Agent transmits 15-minute final records; the central service's planned retention is 15-minute rows for 30 days, hourly summaries for one year, then daily summaries for long-term retention. The central service must define late-arrival, deduplication, recomputation, and ACK semantics. The Agent phase builds only mocks/contract tests of these boundaries, **not** production Bridge, APIs, or PostgreSQL schema.

## 2. Grounded AS-IS baseline

Files inspected on the pinned baseline:

| Real file | Verified AS-IS | Change class |
|---|---|---|
| `main.go` | `cpuSample`, `cpuUsage`, `memoryFrom`, `loadFrom`, `interfacesFrom`, `filesystems`, `snapshot`; CLI `snapshot/stream` writes NDJSON to stdout | KEEP collector behavior; REFACTOR into testable modules |
| `version.go` | v0.1.0; `version --json`; schema/protocol v1.0; capabilities `snapshot`, `stream` | KEEP, extend/version deliberately |
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
- The README's “not yet released” language is stale relative to GitHub Release v0.1.0.
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
| DEC-07 | Daemon-only, **remove version/snapshot/stream --json** from new binary | Versioned legacy Desktop CLI consumer migration; core SSH unchanged |
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

**Approved engineering design defaults:** non-root systemd, one monotonic 1Hz scheduler, one SQLite writer, two bounded independent network workers; initial Live queue 2 events dropping oldest with counted quality; JSON config with atomic reload; locally generated key with time-limited one-use enrollment token; initial backoff 1–60s and healthy failback dwell >=90s as tunable values; v2 daemon wire, immutable retry payload+SHA256, 60-day ACKed cleanup and 61-day unACKed preservation. Exact drivers, hardware numbers and external services are evidence-gated, not magically implemented by these documents.

**Remaining G00 external agreements:** Bridge ingress/wire/subscriber behavior; Persistence verified durable PostgreSQL ACK, same-domain dedup including >30-day late arrivals and hourly/daily reaggregation; PKI issuer/revoke/clone; old Desktop v1 consumer migration; legal license review; QA/Native benchmark rigs. These are **evidence dependencies**, not reopened decisions. **G00 remains BLOCKED** until P00-AC-01–20 and cross-owner sign-off, with implementation authorization separate.

## 4. Work management rules

**Ticket workflow:** `BACKLOG → READY → IN_PROGRESS → REVIEW → VERIFIED → DONE`, or `BLOCKED` with a linked decision.  
**Priority:** P0 correctness, security, durability and gate blockers; P1 lifecycle, compatibility and observability; P2 optional improvements explicitly excluded from the release path.  
**Per-ticket Definition of Ready:** approved source-of-truth/contract; dependent IDs completed; real touched paths identified; negative tests specified; measurable result and rollback plan known.  
**Per-ticket Definition of Done:** implementation/tests/updated docs in one reviewed PR, `go test ./...`, `go vet ./...`, `go test -race ./...` (or documented constrained-platform equivalent), relevant integration/failure tests, passing CI, no unjustified schema/service addition, and evidence linked in ticket. Local green does not override CI red.  
**Evidence:** PR SHA and test command/run URL, fixtures used, test logs or metrics, before/after compatibility samples, security review where applicable. Manual tests require platform, image, steps, expected and observed output.  
**Parallelization:** P01 can proceed after applicable P00 approvals; P02/P03 contracts must precede changes to the data pipeline; P06 can overlap P03–P05 after security/interface approval; P07 and P08 can overlap only after P06 contracts; P09 follows both. P10 preparations may start earlier but must not release prior to P11.

### Phase dependency map

```text
P00 product + architecture + boundary approvals
  ├─> P01 baseline correctness & compatibility
  ├─> P02 schema, SQLite, identity and migrations
  │     └─> P03 1-second sampling & durable recording
  │            └─> P04 minute rollup
  │                   └─> P05 final 15-minute rollup & retention
  └─> P06 secure configuration, registration and mTLS
           ├─> P07 Live channel (also P03)
           └─> P08 Persistence/ACK/replay (also P05)
                  └─> P09 N-endpoint active–standby (also P07)
P01–P09 ──> P10 lifecycle, packaging, security, compatibility
P00–P10 ──> P11 end-to-end qualification & release sign-off
```

## P00 — Architecture, contracts and acceptance baseline (P0)

**Outcome:** implementation-ready agreements without any Agent runtime changes. **Prerequisites:** pinned repository baseline.

| Ticket | Deliverable / precise scope | Verification and acceptance |
|---|---|---|
| AG-0001 | Produce `docs/design/AGENT_PRODUCT_REQUIREMENTS.md` with personas, live/history journeys, non-goals, source-grounded AS-IS vs TO-BE, NFRs and locked invariants. | **Test:** requirement review maps all INV IDs and RTF locked rules 1:1, no contradicting statements. **Accept:** owner approves documented requirements. |
| AG-0002 | Produce `docs/design/AGENT_ARCHITECTURE.md` with component and trust-boundary diagrams, independent channels, CPU/memory/disk/network semantics, UTC lifecycle/state machines, crash invariants and failure matrix. | **Test:** architecture walkthrough of restart, clock jump, disconnected endpoint and overflow; each has deterministic recovery behavior. **Accept:** P02–P09 have no implicit architectural assumption. |
| AG-0003 | Produce `docs/design/AGENT_SQLITE_SCHEMA.md` with DDL for exactly 3 tables, indexes, `PRAGMA user_version` migration plan, transactions and cleanup queries, capacity formula and 60-day policy. | **Test:** paper/SQL review validates unique UTC period IDs, atomic `saved`/rollup behavior and `persisted_at` invariants. **Accept:** data owner signs off. |
| AG-0004 | Produce `docs/design/AGENT_PROTOCOL_AND_SECURITY.md` with N-endpoint config, channel-specific transport, identity/mTLS, authz, version negotiation, final envelope/ACK/error, failover domains, revocation and threat model. | **Test:** mock request/response examples checked against protocol invariants; unauthenticated or unauthorized ACK fails. **Accept:** security and external-contract owners agree. |
| AG-0005 | Produce `docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md` and incorporate this phased backlog as the **single** implementation plan (`docs/design/AGENT_IMPLEMENTATION_BACKLOG.md`); mark known unresolved decisions. | **Test:** every ticket maps to at least one acceptance ID below; negative-path acceptance cannot be “manual later.” **Accept:** owner approves scope and gates; no code is started automatically. |

**Gate G00:** five canonical docs, updated backlog and P00-AC-01–20 evidence explicitly reviewed by product/data/security/QA/external owners, baseline pinned and DEC outcomes recorded. **Current status: BLOCKED pending external sign-off/evidence; no implementation authorization.**

## P01 — Baseline correctness, CLI and testability (P0)

**Outcome:** preserve audited collector semantics, but create **daemon-only v2** (legacy v0.1.0 CLI remains a separate migration artifact). **Prereq:** G00 and DEC-07.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0101 | Extract Linux parsers/collectors from `main.go` with injectable `/proc` fixtures and versioned daemon metrics/quality structs. | Golden collector fixtures pass; new daemon has no requirement to emit legacy CLI JSON. |
| AG-0102 | Correct Linux CPU tick delta accounting (guest duplication, zero/negative deltas, hotplug/reset) and confirm IOWait semantics. | Table-driven synthetic `/proc/stat` tests cover guest, wrap/reset and all-idle; output stays in valid 0–100% ranges, invalid epochs are flagged rather than fabricated. |
| AG-0103 | Surface partial filesystem/network errors, mount/interface parsing problems, permission loss and counter semantics without treating zeros as valid missing readings. | Fixtures for corrupt `/proc/net/dev`, unreadable mounts, escaped mount paths and permission error; observable quality/error fields with versioned compatibility. |
| AG-0104 | Daemon-only signal cancellation, controlled writer drain/rollback and graceful shutdown; no public stream command. | SIGTERM/SIGINT cannot strand partial SQLite transaction, socket, goroutine or service process; no per-second SSH. |
| AG-0105 | Reconcile v0.1.0 release docs, define daemon protocol-major 2 version/health/Hello, protect legacy v1 clients with an explicit migration gate. | Native daemon smoke, old client migration contract, no silent in-place CLI replacement. |

**Gate G01:** AC-001–004 pass; existing v1 CLI consumers protected by **versioned migration**, not by retaining public CLI in the new daemon.

## P02 — SQLite foundation, identity and schema migrations (P0)

**Outcome:** one crash-safe database; no transport or aggregation yet. **Prereq:** G00, DEC-02/04/06/10/11.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0201 | Benchmark and select maintained SQLite Go driver against CGO-free builds, lock handling and all target architectures; record driver and durability ADR. | Build all 4 targets; run native smoke for accessible hosts, identify unqualified targets; microbench WAL write/commit, FULL vs alternatives; results attached. |
| AG-0202 | Implement one DB open/close lifecycle, a controlled writer, busy handling, WAL, approved sync level and integrity/readiness reporting. | Competing readers + one writer under stress cause no deadlock or corruption; injected BUSY returns bounded retry/health; abrupt termination is recoverable. |
| AG-0203 | Create only `server_info`, `raw_samples`, `aggregates`; define keys, indexes, explicit `saved`, interval 60/900, completeness/lineage, checksum/record ID, `persisted_at` final-only. | Schema introspection verifies exactly three application tables, indexes/constraints enforced and invalid resolution/state rejected; no fourth queue table. |
| AG-0204 | Implement schema versioning and forward migration with rollback/backup handling, old-binary incompatibility rules, clean first-install bootstrap. | Empty/new DB, upgrade, failed migration and incompatible downgrade tests; no loss/partial upgrade; PRAGMA integrity test passes. |
| AG-0205 | Persist immutable `agent_id` and host identity/metadata, config association and restore/reinstall policy; ensure cloned machines cannot silently duplicate identity. | Restart preserves identity, clone/re-enroll test rejects collision, wrong-host restore requires explicit recovery approval, no secret leaked via logs. |

**Gate G02:** AC-005–008 pass; DB invariants validated under forced crash, filesystem lock and migration failure.

## P03 — Continuous 1-second sampling and durable raw rows (P0)

**Outcome:** long-running Linux sampler works without Desktop SSH polling. **Prereq:** G01, G02, DEC-03, initial metrics contract.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0301 | Implement one cancellable monotonic scheduler targeting ~1 Hz with UTC timestamps, stable per-sample ID, actual elapsed time and overrun policy. | Fake clock and slow-collector tests show no unbounded goroutine/timer spawn, no duplicate ID, and no invented samples during scheduling gaps. |
| AG-0302 | Collect CPU, RAM, swap, load, mount-level space and interface counters/rates with explicit units, counter reset/wrap, hotplug and cardinality limits. | Linux fixture suite covers reset, wrap, mount/interface disappear/reappear, zero denominator, non-ASCII/escaped mounts, incomplete fields. |
| AG-0303 | Commit one raw row per actual sample, including error/quality/coverage metadata, independent of any network status; control backpressure. | Offline ≥30 min with injected collector errors and network down: stored sample identity/time preserved, explicit missing spans, no memory growth from outbound queues. |
| AG-0304 | Safe daemon startup/shutdown, status and test-clock injection; no old snapshot/stream public interface. | 1-hour soak + SIGTERM + restart verifies monotonic scheduler, no duplicate raw IDs, Desktop migration tracked independently. |

**APPROVED DEC-08 acceptance target:** nominal 1s cadence; schedule-start jitter P99 <=500ms; peak RSS <=128MB; mean CPU <=5% of one core on an explicitly documented reference workload. Benchmark calibration/native-platform qualification remains REQUIRED. Missing samples are visible as `coverage` degradation, never silently filled.

**Gate G03:** AC-009–012 pass; 1-hour workload/partial-data evidence; no SSH reconnection per tick.

## P04 — Transactional UTC 1-minute aggregation (P0)

**Outcome:** trustworthy minute summaries and safe raw cleanup. **Prereq:** G03, DEC-03.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0401 | Define aligned `[UTC_minute, UTC_minute+60s)` source queries; close-window/grace and late-sample behavior. | Boundary tests at xx:xx:00.000 and xx:xx:59.999; DST/timezone and NTP clock-step tests; exact source IDs stable. |
| AG-0402 | Compute per-metric `sample_count`, expected/observed coverage, elapsed-time weighted average where applicable, min/max/last, peak value/time, anomaly summary and per-entity inventory quality. | Golden fixtures with a brief spike show preserved max/peak; 20 missing seconds yield reported 40/60 coverage, not a synthetic 60 samples. |
| AG-0403 | In **one SQLite transaction** upsert/finalize minute row and set `saved=true` only on its exact source raw IDs; ensure idempotency and retry after crash. | Failpoint **before, during and after commit**; restart yields one minute aggregate and all-or-none saved flags. No sample is marked saved without durable minute. |
| AG-0404 | Delete **only** already-saved raw rows in bounded batches; permit cleanup to resume across shutdown and busy errors. | Verify unsaved row survives expired staging age, crash during DELETE, disk pressure and concurrent reader; data loss count = 0 for unsaved records. |

**Gate G04:** AC-013–017 pass, including atomic crash-failpoint tests and spike/coverage golden files.

## P05 — Transactional 15-minute finalization, queue and 60-day retention (P0)

**Outcome:** self-contained finalized historical rows, safe local replay queue. **Prereq:** G04, DEC-03/04.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0501 | Aggregate 15 finalized minute windows into `[quarter_hour, quarter_hour+900s)` with semantically correct weighted stats, extrema/peak timestamps, anomaly union and partial coverage. | Simulate a 1-second peak within sparse minute windows; final max is retained and weighted average respects actual observations. |
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
| AG-0701 | Define Live sample envelope: IDs, UTC, actual interval, schema, quality/errors, monitored host identity, metrics, bounded size and heartbeat. | Contract golden files round-trip on target schema; 1 Hz messages validate; malformed/mismatched identity rejected by mock. |
| AG-0702 | Add bounded non-durable outbound live buffer, explicit drop/backpressure metrics and independent goroutine lifecycle. | Slow/blocked mock Bridge cannot delay collector commit or minute/final rollups; bounded memory; dropped live events counted. |
| AG-0703 | Implement reconnect and heartbeat with timeout/jitter, no per-second SSH or connection open/close; backfill semantics explicitly **not** historical replay. | Force 10 disconnections and recovery; no runaway goroutines/socket count; UI gets new live samples after reconnection; no fabricated historical stream. |
| AG-0704 | Provide mock Bridge contract tests that relay to a live subscriber and contain **no DB/history reads/writes**. | Test double assertions: no historical query or storage interface exercised; offline Persistence still permits live relay. |

**Gate G07:** AC-028–031 pass; live soak with blocked bridge demonstrates collection/storage independence.

## P08 — Durable Persistence channel, ACK and replay (P0)

**Outcome:** final records reach *only* the Persistence API and are acknowledged by proof of durable, idempotent central commit. **Prereq:** G05, G06, DEC-01/03/04/05.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0801 | Define submission envelope: stable record ID, agent/host identity, UTC start, 900s resolution, schema version, completion/coverage, checksum/content hash, payload and retry identity. | Mock API rejects raw/60s data, wrong host/period, malformed checksum, non-finalized and unauthorized rows. |
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

## P10 — Unattended operations, security, packaging, migration (P0/P1)

**Outcome:** manageable Linux service without changing Desktop core SSH. **Prereq:** G01–G09, DEC-07/09/10.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-1001 | Daemon-only entrypoint and safe install/uninstall/systemd service, journald health, liveness and deterministic drain/restart. | systemd restart/stop/kill leaves no half-committed sample/orphan process/privilege escalation/public listener. |
| AG-1002 | Least-privilege runtime user, controlled DB/config/cert paths, strict key permissions, refusal of unsafe config/cert ownership, log redaction and resource limits. | Non-root smoke; permission/owner attacks rejected; secrets never printed, world-writable key/config rejected. |
| AG-1003 | Versioned daemon-v2 migration/upgrade/rollback for binaries, schema, certs and legacy SSH-installed v0.1.0 coexistence. | Unsupported downgrade blocks; no silent incompatible replacement for old Desktop CLI; existing SSH core unchanged. |
| AG-1004 | Sign/check release provenance and artifact digests; update tar/deb/rpm for approved service layout, new deps and correct license/copyright/NOTICE. | Independently verify trusted signature and SHA, package metadata, install/upgrade/uninstall on native supported OS, no leftover service state after safe uninstall. |
| AG-1005 | Expand CI to protocol, failpoint, race, security, packaging, signed release and **native** ARMv6/ARMv7/ARM64 qualification (lab or controlled emulation clearly labeled). | Cross-compiles all targets; native reference runs recorded separately; unsupported targets explicitly excluded/marked experimental before publish. |
| AG-1006 | Complete docs: install, registration, recovery, trust/rotation, protocol versions, resource/sizing, runbook, licensing; preserve changelog of migration. | Fresh-user documentation drill from bare Linux VM to observed live + durable historical mock ACK, with no missing secret steps. |

**Gate G10:** AC-045–051 pass; real deployment rehearsal on disposable hosts only, security/license approvals attached. Production deployment is a separate authorization.

## P11 — Full acceptance, resilience, performance and release (P0)

**Outcome:** evidence-backed release candidate and closed Agent scope. **Prereq:** all prior gates.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-1101 | Run full deterministic data-integrity/failure-injection matrix: crash before/during/after minute and 15m commits, power loss/WAL, DB busy/corruption, full disk, clock jumps, network loss and ACK loss. | All INV-01–10 hold; every data gap/error is observable; no acknowledged false success or silently removed unsaved/pending rows. |
| AG-1102 | Run multi-host and N=3 dual-channel E2E with separate Bridge mock and durable central API mock, replay, shared/non-shared domains and recovery from 60+ days offline. | Final-only 900s ingestion, live 1s relay, independent channels, stable IDs, idempotent central records and blocked unsafe domain failover confirmed. |
| AG-1103 | Run sustained reference benchmarks and 60-day accelerated virtual-time aging for CPU/RSS/SQLite/WAL/retention, replay catch-up and high-cardinality mount/interface inventories. | Measurements with hardware/OS/tooling included; meets approved DEC-08 targets or fails release gate with a tracked mitigation. |
| AG-1104 | Verify platform matrix for Linux amd64, arm64, armv7, armv6: build, native smoke, service, network auth, SQLite integrity and artifact authenticity. | Every advertised production platform passes native qualification; unqualified platforms are not claimed as production-supported. |
| AG-1105 | Review security (spoof/replay/token/cert/ACL/key exposure), docs, migration/restore, license provenance, API contract owners and all linked evidence; freeze Agent scope. | All P0/P1 blockers closed, owner checklist signed; versioned tag/release is authorized separately, no unreviewed runtime/API behavior. |

**Gate G11 — Release candidate acceptance:** all signed INV + AC criteria, mandatory CI and native matrix, approved quantitative SLOs, release audit, owner sign-off; **only then** tag/release under separately approved execution authority.

## 5. Stable acceptance IDs — canonical test matrix

The **authoritative Given/When/Then acceptance text** is [AGENT_ACCEPTANCE_AND_TEST_MATRIX.md](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md). This backlog references its stable IDs, so duplicate changing test definitions are deliberately removed here. **AC-003, AC-004, AC-047** now reflect approved DEC-07 daemon-only; P00-AC-01–20 are the design gate.

| IDs | Phase | Coverage |
|---|---|---|
| P00-AC-01–20 | P00 | Design/docs/external agreement sign-off |
| AC-001–004 | P01 | Collector/daemon/legacy migration |
| AC-005–008 | P02 | SQLite/identity/recovery |
| AC-009–012 | P03 | 1Hz sampler |
| AC-013–017 | P04 | 1m+saved atomicity |
| AC-018–022 | P05 | 15m/60d/61d retention |
| AC-023–027 | P06 | mTLS/config/authorization |
| AC-028–031 | P07 | stateless live bridge |
| AC-032–038 | P08 | durable ACK/replay |
| AC-039–044 | P09 | N endpoint HA |
| AC-045–051 | P10 | systemd/signed native packages/migration |
| AC-052–055 | P11 | E2E/fault/performance/release |

Negative fixture inventory and explicit G00–G11 evidence/sign-off are maintained only in the canonical acceptance matrix.

## 6. Boundary contracts the Agent needs; external services remain out of scope

The following are **contract tests against mocks** in this repo. They are not permission to implement another service.

| Contract | Minimum external guarantee |
|---|---|
| Live Bridge | Authenticated + authorized inbound Agent stream, subscribed UI forwarding, no durable history or historical query route; close/error/heartbeat semantics |
| Persistence API | Authenticated + authorized `agent_id`, **900s final-only** ingestion, stable-ID idempotent durable PostgreSQL commit, ACK containing verifiable ID/hash/domain, typed errors and retryability |
| Historical API | UI historical reads reflect durable central records; never bridge/Agent SQLite; late-arrival correction and central 30-day/1-year/long-term policies documented by their owner |
| Monitoring registration/PKI | Trusted certificate authority, server identity, Agent enrollment/rotation/revocation, allowed endpoint/channel mappings; controlled restoration |
| Desktop/SSH legacy | Legacy v0.1.0 may continue for existing consumers pending explicit versioned migration. **New daemon removes public v1 CLI**, must never silently replace old consumers; core SSH/vault unchanged. |

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

- [ ] DEC-01–11 approved; P00-AC-01–20 and G00 external contract/QA evidence explicitly signed.
- [ ] G01–G10 and AC-001–055 verified with linked evidence.
- [ ] No unknown “partial success” for raw saved, minute final, 15m final or central durable ACK.
- [ ] All advertised platforms native-tested; CPU, RSS, disk/WAL, jitter and catch-up thresholds approved and met.
- [ ] N=3 separate-channel failover/failback and ACK-domain test passes.
- [ ] No history through Bridge, no raw/minute to Persistence API, no PostgreSQL directly from Agent.
- [ ] 60-day final history and 61-day pending/recovery policy proved without silent data loss.
- [ ] Negative mTLS/authz, replay, clone and cert rotation cases pass.
- [ ] Daemon-v2 migration evidence protects legacy CLI v1 consumers; no incompatible silent replacement; Desktop SSH remains untouched.
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
Rollback / compatibility: <strategy and old-client behavior>
Definition of Done: <CI link, tests, logs, review approvals, documentation>
```

## 9. Source-of-truth and change control

1. **Observed AS-IS:** pinned source commit and verified test/run evidence override stale README claims.
2. **Locked TO-BE:** owner-supplied Agent architecture guide takes priority over legacy implementation, unless explicitly re-approved.
3. **Approved TO-BE design:** `AGENT_PRODUCT_REQUIREMENTS.md` is canonical for product decisions; `AGENT_ARCHITECTURE.md` for design boundaries; `AGENT_SQLITE_SCHEMA.md` and `AGENT_PROTOCOL_AND_SECURITY.md` for their respective contracts; `AGENT_ACCEPTANCE_AND_TEST_MATRIX.md` for test requirements.
4. **This file** is the **single execution backlog and implementation sequence**, not an alternative system architecture. Do not create a redundant `AGENT_IMPLEMENTATION_PLAN.md`. Change IDs or gate criteria only with documented owner approval and linked impact analysis.
5. Require small PRs, review of real file/line impacts, explicit version/migration handling and evidence. This file is documentation; **no Agent code, repository release, service or production server changes are authorized by it**.

**Backlog completeness criterion:** Every mandatory product behavior has a ticket, acceptance ID, dependency, negative-path test and gate. Anything needed to ship but missing here is a scope blocker to add through reviewed backlog change—not an implicit “developer choice.”
