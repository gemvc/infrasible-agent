# Infrasible Monitoring Agent — Phased Implementation Backlog

**Status:** PROPOSED / NOT APPROVED FOR IMPLEMENTATION  
**Scope:** standalone open-source Linux Agent and the *minimum* external boundary contracts needed to test it  
**Repository:** https://github.com/gemvc/infrasible-agent  
**Baseline:** `main` at `2f93988bf37243d7a7d57d1a4d4172bec1b8302c` (inspected 2026-10-18)  
**Baseline product:** `agentVersion=0.1.0`, `schema_version=1.0`, `protocol 1.0`  
**Document owner:** Agent architecture/product owner (approval required); execution by assigned engineering and QA owners  
**Change policy:** This backlog is documentation only. It neither approves implementation nor authorizes changes to Desktop, Bridge, Persistence service, PostgreSQL, or production hosts.

> **How to use this file:** Complete P00 and obtain explicit architecture sign-off before writing Agent implementation code. Implement subsequent phases in dependency order using one small PR per deliverable or logically indivisible set. A phase is **done** only when its evidence and gate are accepted. “Implemented” without tests and failure evidence is **not done**. Entries labeled **PROPOSED** are not product commitments.

## 1. Product destination and non-negotiable architecture

The Agent runs **on each monitored Linux host**, samples host metrics approximately once per second, stores monitoring state in a **single SQLite database with exactly three logical tables**, and performs UTC 1-minute then UTC 15-minute rollups. Final 15-minute aggregates are locally retained for 60 days, subject to an explicitly approved pending/expiry policy. The Agent initiates outbound connections; there is no periodic reopening of SSH to collect live samples.

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
| INV-10 | The existing proven collector and public CLI contracts are audited before alteration; proprietary Desktop SSH implementation and credentials are out of scope. |

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

## 3. Decision register — P00 is a hard stop

Decisions in Section 1 are LOCKED. The entries below are **unresolved** and must receive an explicit recorded verdict before dependent implementation. Recommendations are proposals, not pre-approved designs.

| ID / owner | Requires approval | PROPOSED default; reason / dependent phases |
|---|---|---|
| DEC-01 / product + protocol | Live transport and Persistence API envelope, protocol major, endpoints, and compatible schema negotiation | Agent-initiated mTLS stream for Live and mTLS HTTP request/response with keep-alive for Persistence; specify exact transports only after contract review. Blocks P06–P09. |
| DEC-02 / security + product | Initial enrollment, trust roots, cert issuance/rotation/revocation, re-enrollment and key storage | One stable `agent_id` in `server_info`; explicit per-server authorization and OS-protected identity material; enrollment token remains *proposal*. Blocks P06. |
| DEC-03 / data + product | Freeze point, late-sample grace, corrections to finalized/ACKed periods, clock corrections | Immutable period after a bounded, documented grace; reject/quarantine late entries with health evidence; never silently mutate ACKed payload. Grace (e.g. 120 s) is **PROPOSED**, not locked. Blocks P04–P08. |
| DEC-04 / product + ops | 60-day expiration of **unACKed** final rows, disk-full behavior | Never silently delete unACKed data; prefer retention exception + urgent observable alarm and explicit operator policy over invisible data loss. Define finite disk safety behavior. Blocks P05, P08, P10. |
| DEC-05 / data/platform | Meaning of failover among endpoints with distinct backing persistence domains | Automatic Persistence failover only within a proven shared/deduplicated central domain, unless an explicit cross-domain replication/ACK contract exists. Live failover remains independent. Blocks P08–P09. |
| DEC-06 / Go + platform | Maintained SQLite driver, `CGO_ENABLED=0` support across ARMv6/v7 and durable sync policy | Benchmark a maintained pure-Go driver versus alternatives; start with WAL + `synchronous=FULL` for durability evaluation, not an unmeasured commitment. Blocks P02. |
| DEC-07 / product + Desktop owner | Whether current `version/snapshot/stream --json` must remain indefinitely, be opt-in or deprecate | Preserve CLI v1 output unchanged in initial migration; add daemon mode separately, deprecate only by explicit versioned policy. Blocks P01/P10. |
| DEC-08 / product + QA | Quantitative cadence, CPU/RSS/disk, jitter, replay throughput SLOs and reference hardware | Define reproducible measurement harness, workloads and environment, then approve thresholds from observations rather than presenting guesses as guarantees. Blocks final P11 release gate. |
| DEC-09 / security + legal | Apache-2.0 transition, provenance, license/NOTICE requirements and supply-chain signing model | Audit copyright and third-party notices; introduce verifiable signed artifacts before unattended updates. Blocks P10–P11. |
| DEC-10 / platform + ops | SQLite corruption and host-restore identity policy; backup and rollback support | Fail closed/emit health on corruption, avoid overwriting valid state; same-host restore must preserve identity intentionally, cloned hosts must not share `agent_id`. Blocks P02/P10. |
| DEC-11 / protocol + product | Meaning of `server_info` vs N registered Monitoring endpoints | `server_info` identifies one monitored host/agent; authorized N endpoints and their order live in one versioned config file, not separate application tables. Blocks P02/P06. |

**P00 completion rule:** no “TBD” contract is silently decided by code. Every unresolved choice has an ADR/owner approval, or an explicitly scoped implementation dependency remains BLOCKED. If one decision is still open, affected phases cannot begin.

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

**Gate G00:** signed requirements + architecture + protocol + storage + acceptance contract, explicit DEC dispositions, baseline SHA captured. Deliverable-only changes are allowed by a separately authorized documentation task; implementation still requires separate authorization.

## P01 — Baseline correctness, CLI and testability (P0)

**Outcome:** preserve working v0.1.0 collector while correcting evidence-backed defects. **Prereq:** G00 and DEC-07.

| Ticket | Deliverable / precise scope | Tests and acceptance |
|---|---|---|
| AG-0101 | Extract parsers/collectors from `main.go` into internal Go packages using injectable `/proc` fixtures; keep snapshot JSON shape stable. | Golden v1 JSON fixtures for `version/snapshot/stream` pass; existing command exit codes and stderr/stdout conventions do not change. |
| AG-0102 | Correct Linux CPU tick delta accounting (guest duplication, zero/negative deltas, hotplug/reset) and confirm IOWait semantics. | Table-driven synthetic `/proc/stat` tests cover guest, wrap/reset and all-idle; output stays in valid 0–100% ranges, invalid epochs are flagged rather than fabricated. |
| AG-0103 | Surface partial filesystem/network errors, mount/interface parsing problems, permission loss and counter semantics without treating zeros as valid missing readings. | Fixtures for corrupt `/proc/net/dev`, unreadable mounts, escaped mount paths and permission error; observable quality/error fields with versioned compatibility. |
| AG-0104 | Add explicit process cancellation/signal handling with deterministic shutdown for stream/daemon paths; test leaks and JSON output integrity. | SIGTERM/SIGINT test leaves no partial JSON line or goroutine leak; no release of SSH session every second. |
| AG-0105 | Reconcile README release/supported targets and introduce semver/protocol compatibility rules; preserve v1 CLI unless DEC-07 says otherwise. | Native amd64 CLI smoke; version/stream consumer contract golden tests; docs match tag v0.1.0 and packaging outputs. |

**Gate G01:** AC-001–004 pass; baseline CLI consumers remain functional or owner-approved versioned migration exists.

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
| AG-0304 | Provide safe startup/shutdown, status and test clock injection; decide integration with legacy `snapshot` and `stream`. | 1-hour soak + SIGTERM + restart verifies monotonic scheduler, no duplicate raw IDs, CLI v1 contract unaffected. |

**PROPOSED performance target, not approved SLO:** nominal 1s cadence, with quantile/jitter target and test host calibrated in P00. Missing samples are visible as `coverage` degradation, not silently filled.

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
| AG-1001 | Add explicit long-running service CLI and safe install/uninstall/service supervision (systemd where available), logging, readiness/liveness, exit semantics, graceful draining and restart behavior. | systemd restart/stop/kill tests show no lost committed sample, no orphan goroutine/process, no privilege escalation or network listener unexpectedly exposed. |
| AG-1002 | Least-privilege runtime user, controlled DB/config/cert paths, strict key permissions, refusal of unsafe config/cert ownership, log redaction and resource limits. | Non-root smoke; permission/owner attacks rejected; secrets never printed, world-writable key/config rejected. |
| AG-1003 | Versioned upgrade/rollback plan for binaries, SQLite schema, cert rotation, stable identity and current SSH-installed package coexistence. | Update interrupted mid-flight and rollback test; unsupported downgrade blocks safely; existing CLI clients remain compatible per DEC-07. |
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

## 5. Stable executable acceptance matrix

Every item below is a mandatory **release-level** assertion, not just a design aspiration. File-specific automation, fixtures and run links belong in its implementing ticket. Labels **UT**=unit, **IT**=integration, **FT**=fault injection, **CT**=contract, **E2E**=end-to-end, **NT**=native/soak/performance, **SEC**=security.

| AC ID | Required Given / When / Then and pass condition | Type / primary phase |
|---|---|---|
| AC-001 | Given baseline Linux fixtures, when parsers run, then valid CPU/memory/load/disk/net values and explicit partial errors are emitted, never silently replaced with valid zero. | UT P01 |
| AC-002 | Given Linux guest ticks and counter reset, when CPU usage is derived, then guest is not double-counted; invalid deltas are quality-flagged; range remains physically valid. | UT P01 |
| AC-003 | Given v1 consumer fixtures, when `version/snapshot/stream --json` run, then compatible JSON, stdout/stderr split and exit behavior are preserved. | CT P01 |
| AC-004 | Given SIGTERM/SIGINT in stream mode, when cancelled, then process finishes promptly without resource leaks or truncated JSON. | IT P01 |
| AC-005 | Given a fresh DB, when initialized, then the only logical Agent tables are `server_info`, `raw_samples`, `aggregates` with approved indexes/constraints. | IT P02 |
| AC-006 | Given SQLite busy/crash/WAL checkpoint stress, when reopened, then integrity is OK and there is no partially committed logical operation. | FT P02 |
| AC-007 | Given upgrade/failed migration/downgrade attempts, when schema version rules execute, then upgrade is atomic and unsafe binary/schema mismatch fails closed. | FT P02 |
| AC-008 | Given restart/clone/restore, when identity loads, then intended identity is stable, host collision detected and secrets undisclosed. | IT/SEC P02 |
| AC-009 | Given a healthy reference host and fake clock, when sampling runs, then nominal cadence is ~1 second with recorded actual elapsed time and no invented samples. | NT/UT P03 |
| AC-010 | Given missing/partial reads, counter resets and mount/interface churn, when sampled, then error/coverage semantics are correct per metric and entity. | UT P03 |
| AC-011 | Given absent networks/Bridge/API, when sampling continues, then raw rows persist independently with bounded memory and observable gaps. | IT/FT P03 |
| AC-012 | Given a 1-hour run and restart, when resumed, then no duplicate raw IDs, runaway timers or per-second SSH reconnects occur. | NT P03 |
| AC-013 | Given time xx:xx:00.000/59.999, when minute closes, then `[start,end)` window includes each valid raw ID exactly once. | UT P04 |
| AC-014 | Given 40 observed samples/60 expected plus a short spike, when aggregated, then reported coverage is 40/60 and peak/max are retained; missing 20 are not invented. | UT P04 |
| AC-015 | Given crash before/during minute transaction, when reopened, then both final minute and `saved` changes are all-or-none. | FT P04 |
| AC-016 | Given retry after completed minute transaction, when rerun, then no duplicate minute record or mis-marked raw ID is created. | FT P04 |
| AC-017 | Given saved and unsaved raw rows, when cleanup runs or crashes, then only saved rows are deleted. | FT P04 |
| AC-018 | Given 15 completed/partially missing minutes, when 900s rollup runs, then coverage, count, weighted values, extrema, last and anomaly evidence remain mathematically correct. | UT P05 |
| AC-019 | Given crash around 15m commit/cleanup, when retried, then one final record exists and no required minute is lost before commit. | FT P05 |
| AC-020 | Given pending final records inserted out-of-order, when queried, then oldest unACKed 900s period is returned first. | IT P05 |
| AC-021 | Given 61 virtual days including unACKed old data, when retention runs, then ACKed data follows approved expiry policy and unACKed data is never silently dropped. | FT P05 |
| AC-022 | Given measured reference cardinality, when 60-day storage simulation runs, then DB/WAL sizing and cleanup comply with approved budget with documented assumptions. | NT P05 |
| AC-023 | Given config with N=1/3/10, when loaded, then priority equals array index, no fixed two-server constraint, unsupported versions rejected. | UT P06 |
| AC-024 | Given bad/untrusted/expired/revoked cert or wrong server name, when dialled, then no operational mTLS channel forms and there is no plaintext fallback. | SEC P06 |
| AC-025 | Given authenticated but unauthorized Agent/channel, when Hello or send executes, then authorization is denied with typed non-secret diagnostic. | SEC/CT P06 |
| AC-026 | Given certificate/key rotation during service life, when rotated, then new trusted identity reconnects and old revoked identity cannot transmit. | SEC/IT P06 |
| AC-027 | Given incompatible major protocol and oversized payload, when negotiating, then fail closed/bounded with no unbounded allocation. | SEC/CT P06 |
| AC-028 | Given a subscribed mock Bridge, when healthy 1s samples arrive, then subscribers receive ordered/schema-valid live envelopes; no historical DB read/write occurs. | CT/E2E P07 |
| AC-029 | Given a permanently slow Bridge, when collection continues, then sample writes and rollups remain independent; live buffer stays bounded with explicit drops. | FT/NT P07 |
| AC-030 | Given network disconnection and re-establishment, when stream reconnects, then fresh live data resumes without fabricated historical backfill or per-tick SSH. | IT P07 |
| AC-031 | Given Persistence API down, when Live Bridge stays up, then live samples continue through Bridge without waiting for central ACK. | E2E P07 |
| AC-032 | Given raw or minute payload to the Persistence mock, when submitted, then protocol rejects it; only finalized 900s records qualify. | CT P08 |
| AC-033 | Given a 200 response without authenticated durable ACK, when handled, then `persisted_at` stays NULL. | FT/CT P08 |
| AC-034 | Given valid durable ACK for the same agent/period/ID/hash/domain, when verified, then only the matching final row gets `persisted_at` exactly once. | IT/SEC P08 |
| AC-035 | Given success at central commit but lost ACK, when resubmitted across restart, then exactly one central logical record exists. | FT/E2E P08 |
| AC-036 | Given wrong hash, unknown agent or forged/domain-mismatched ACK, when received, then state never advances to persisted. | SEC/CT P08 |
| AC-037 | Given throttling, timeout and out-of-order database rows, when replayed, then retries are bounded with jitter and oldest-first order preserved. | IT P08 |
| AC-038 | Given 1/30/61 days offline, when running simulated recovery, then age/backlog/capacity warnings and non-silent loss policy work and collection continues. | FT/NT P08 |
| AC-039 | Given N=3 mixed health/capabilities, when selecting per channel, then exactly one highest-priority eligible endpoint per channel is active. | IT P09 |
| AC-040 | Given primary outage and recovery, when failover/failback occurs, then configured backoff and hysteresis prevent connection flapping. | FT P09 |
| AC-041 | Given Live primary down but Persistence primary healthy (and reverse), when operating, then two channels may select different servers without interference. | E2E P09 |
| AC-042 | Given shared-domain endpoints, when ACK disappears and failover occurs, then deduplication stays stable across both paths. | E2E P09 |
| AC-043 | Given **independent** persistence domains, when a failover would imply false availability of Server A's data on B, then unsafe selection/ACK assumption is blocked and observable. | SEC/FT P09 |
| AC-044 | Given endpoint revocation/reorder under load, when config reloads, then revoked authority is not reused and resources are bounded. | SEC/IT P09 |
| AC-045 | Given systemd stop/restart/crash on disposable Linux, when restarted, then local SQLite remains valid, service enters healthy state and no orphan process stays. | NT/FT P10 |
| AC-046 | Given non-root runtime with malicious file permissions, when opening config/cert/DB, then protected paths are enforced and no secret is logged. | SEC P10 |
| AC-047 | Given interrupted upgrade/rollback and old CLI client, when migrating, then schema safety and approved v1 compatibility rules hold. | FT/CT P10 |
| AC-048 | Given release artifacts, when checked independently, then signed provenance and checksums verify against trusted publisher; invalid signature fails. | SEC P10 |
| AC-049 | Given build targets amd64/arm64/armv7/armv6, when CI completes, then compilation succeeds and native runtime results/unsupported caveats are reported separately. | NT P10 |
| AC-050 | Given package install/upgrade/uninstall on supported OS, when operated as service, then files/identity/permissions behave as documented and no unsafe replacement occurs. | NT P10 |
| AC-051 | Given a clean Linux VM and only public docs, when operator follows onboarding/recovery guide, then can collect data and connect to contract mocks without undocumented steps. | E2E P10 |
| AC-052 | Given full crash/corrupt/disk-full/clock/ACK-loss fault suite, when replayed, then INV-01–10 remain true or an approved explicit degraded/error state is exposed. | FT P11 |
| AC-053 | Given N=3 multi-host test rig, when both channels run with independent outages, then 1s Live and finalized 900s Persistence match their distinct approved APIs. | E2E P11 |
| AC-054 | Given accelerated 60-day dataset and approved reference host, when stressed, then CPU/RSS/DB/WAL/jitter/recovery throughput meet owner-approved measured SLOs. | NT P11 |
| AC-055 | Given candidate release, when architecture, QA, security, licensing and external-interface owners review evidence, then all P0/P1 release blockers close and sign-off is explicit. | Review P11 |

### Cross-cutting fault matrix / mandatory fixtures

| Fault family | Mandatory injection | Preserved invariant / evidence |
|---|---|---|
| Transaction crash | Force stop before, mid, after minute and final DB transactions | No half-aggregation and no invalid `saved`; idempotent restart |
| Abrupt power loss / WAL | Kill -9, filesystem interruption in disposable VM, checkpoint during write | DB integrity or explicit corruption alert; no false success |
| Storage exhausted / corruption | ENOSPC, permission denied, corrupt DB, failed backup/restore | Safe degraded state, alert, no silent overwrite or clean-away |
| Collection quality | Partial reads, counter wrap/reset, hotplug, 1000 interfaces/mounts | Bounded memory, explicit quality and cardinality limits |
| Clock anomaly | NTP backward/forward, timezone/DST, delayed sampler | UTC boundary integrity, no duplicate period ID or invented data |
| Transport | Stall, packet loss, high latency, reconnect, large messages | Bounded retry/queue/CPU, samples continue |
| Identity/authz | Bad CA, hostname, SNI, cert expiry/revoke, removed ACL | Denied session and no unauthorized ACK |
| Central ACK | Lost, late, forged, wrong ID/hash/domain, duplicate | `persisted_at` only for valid durable commit; dedup on replay |
| Endpoint HA | N=3 down/up/flapping, server capability mismatch, domain split | Independent highest-priority eligible per channel; no split ACK claim |
| Retention/backlog | 1/30/61+ days disconnected, restart during cleanup | Approved expiry and non-silent old pending behavior |
| Rollout | Failed migration, incompatible downgrade, stolen key, host clone | Block unsafe launch, preserve identity/DB and diagnostics |

## 6. Boundary contracts the Agent needs; external services remain out of scope

The following are **contract tests against mocks** in this repo. They are not permission to implement another service.

| Contract | Minimum external guarantee |
|---|---|
| Live Bridge | Authenticated + authorized inbound Agent stream, subscribed UI forwarding, no durable history or historical query route; close/error/heartbeat semantics |
| Persistence API | Authenticated + authorized `agent_id`, **900s final-only** ingestion, stable-ID idempotent durable PostgreSQL commit, ACK containing verifiable ID/hash/domain, typed errors and retryability |
| Historical API | UI historical reads reflect durable central records; never bridge/Agent SQLite; late-arrival correction and central 30-day/1-year/long-term policies documented by their owner |
| Monitoring registration/PKI | Trusted certificate authority, server identity, Agent enrollment/rotation/revocation, allowed endpoint/channel mappings; controlled restoration |
| Desktop/SSH legacy | May continue v1 `version/snapshot/stream` consumption until an explicit migration plan; core SSH/vault implementation is not changed in this project |

**Proposed wire example (illustrative only, not approved):**

```json
{
  "message_type": "final_aggregate",
  "protocol_major": 2,
  "agent_id": "<stable-agent-id>",
  "record_id": "<stable-agent-and-utc-period-derived-id>",
  "period_start_utc": "2026-10-18T00:00:00Z",
  "period_seconds": 900,
  "schema_version": "<approved-schema>",
  "final": true,
  "coverage": { "observed_samples": 893, "expected_samples": 900 },
  "payload_hash": "<canonical-payload-hash>",
  "metrics": {}
}
```

```json
{
  "message_type": "durable_ack",
  "agent_id": "<stable-agent-id>",
  "record_id": "<same-record-id>",
  "period_seconds": 900,
  "payload_hash": "<same-canonical-payload-hash>",
  "persistence_domain_id": "<approved-domain>",
  "durably_committed": true,
  "committed_at_utc": "<server-commit-timestamp>"
}
```

The envelope, sample count and expected count above are **examples**, not a claim that all metrics have 900 observations. Explicit contract approval is required before implementation. A positive envelope from a malicious/untrusted sender is never valid without authenticated and authorized transport/context.

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

- [ ] P00 signed; DEC-01–11 resolved, or any unresolved non-release scope explicitly excluded by owner.
- [ ] G01–G10 and AC-001–055 verified with linked evidence.
- [ ] No unknown “partial success” for raw saved, minute final, 15m final or central durable ACK.
- [ ] All advertised platforms native-tested; CPU, RSS, disk/WAL, jitter and catch-up thresholds approved and met.
- [ ] N=3 separate-channel failover/failback and ACK-domain test passes.
- [ ] No history through Bridge, no raw/minute to Persistence API, no PostgreSQL directly from Agent.
- [ ] 60-day final history and 61-day pending/recovery policy proved without silent data loss.
- [ ] Negative mTLS/authz, replay, clone and cert rotation cases pass.
- [ ] Existing SSH-v1 CLI compatibility/migration evidence present; Desktop SSH remains untouched.
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
