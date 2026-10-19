# Infrasible Monitoring Agent — Acceptance and Test Matrix

> **Note:** Target/acceptance text below. For current shipped behavior and verdict see [STATUS.md](../STATUS.md), [README](../../README.md), and operator docs under `docs/`. Relay builders: [MONITORING_RELAY.md](MONITORING_RELAY.md).

**Status:** source-grounded acceptance baseline approved for documentation; **NOT EXECUTED** for new architecture. Release requirements are not test results. Existing executable baseline covers only legacy v0.1.0 code/CI.
**Owner decisions:** DEC-01–DEC-11 and final owner-approved refinements **AD-01 (multi-rate), AD-02 (quality/freshness), AD-03 (lean daemon/early health)** on 2026-10-18. **Repository reference:** `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed`.
**Source of truth for acceptance:** THIS file; `AGENT_IMPLEMENTATION_BACKLOG.md` is the single phased execution plan and refers to stable IDs here.

## 1. Test levels and proof format

`UT` unit, `IT` integration, `FT` fault injection, `CT` external contract/mock, `E2E` end-to-end with mock/safe disposable services, `NT` native soak/performance, `SEC` security, `REV` review, `LEGAL` license review. A PASS requires recorded environment/image/arch, exact revision, fixture version, command, expected and observed result, relevant CI run or evidence artifact, responsible reviewer, and any remaining limitation. Code and live central-service behavior are **not** claimed implemented just because their mock tests pass.

## 2. P00 design acceptance (must close before code)


| P00 AC | Given / When / Then | Type |
|---|---|---|
| P00-AC-01 | Given owner DEC-01–11 and AD-01–03, when checked across all canonical docs and backlog, then 1Hz frames with independent metric cadences/quality are consistent with all locked decisions, including unchanged 120s grace. | REV P00 |
| P00-AC-02 | Given current baseline SHA and v0.1.0 code, when inventory is reviewed, then AS-IS is separated from all proposed daemon behavior. | REV P00 |
| P00-AC-03 | Given zero users and clean installation, when auditing P01/P10, then legacy CLI and Desktop migration requirements are removed and only the new daemon contract is tested. | CT P00 |
| P00-AC-04 | Given proposed DDL, when schema reviewed, then exactly three application tables exist and frame-level slots are distinguished from per-metric 1s/5s/30s opportunities stored within JSON, with enforceable transactional invariants. | SQL P00 |
| P00-AC-05 | Given a crash before/mid/after 60s and 900s transactions, when state transitions are paper-walked, then saved/rollup/persisted predicates remain atomic and idempotent. | FT-REVIEW P00 |
| P00-AC-06 | Given UTC 120s grace, clock rollback and mixed 1s/5s/30s metric schedules, when a full minute, partial minute and 15m are simulated, then immutable boundaries, schedule-phase quality and metric-specific denominators are exact; no cached value is fabricated as fresh. | UT-REVIEW P00 |
| P00-AC-07 | Given 61-day offline and ENOSPC, when retention reviewed, then unACKed rows are never silently deleted and lost new samples are reported honestly. | FT-REVIEW P00 |
| P00-AC-08 | Given slow/disconnected Bridge and 30s-old cached capacity, when Live contract reviewed, then bounded drops, fresh-frame delivery, preserved original metric timestamp/age/stale status, no new cached observation and no historical storage are explicit. | CT P00 |
| P00-AC-09 | Given HTTP 200 without valid durable ACK, when protocol reviewed, then persisted_at remains NULL and stable replay ID/hash are required. | CT P00 |
| P00-AC-10 | Given central commit-success/ACK-loss, when mock semantics reviewed, then only one logical central final and an authenticated domain-bound ACK can complete local state. | CT P00 |
| P00-AC-11 | Given N=1/3/10 with mixed permissions, when selection documented, then priority is array order, one active per channel and independent endpoint choice. | CT P00 |
| P00-AC-12 | Given two independent persistence domains, when failover documented, then Agent cannot treat shared config string or prior ACK as cross-domain proof. | SEC P00 |
| P00-AC-13 | Given cert expiration/revocation/clone, when trust flow documented, then fail closed, re-enroll and identity handling are explicit. | SEC P00 |
| P00-AC-14 | Given SQLite corruption/host restore, when operator workflow documented, then no auto-reset and same-host versus clone identity is unambiguous. | FT-REVIEW P00 |
| P00-AC-15 | Given DEC-08 and 1s/5s/30s plus change-driven metadata cadences, when benchmark harness reviewed, then RSS/CPU/jitter/mount cardinality, due-rate correctness, collector overhead, disk/WAL and native arch conditions are reproducible. | NT-REVIEW P00 |
| P00-AC-16 | Given 30-day central 15m retention and 61-day Agent Pending, when external API contract reviewed, then idempotent late acceptance/hourly-daily recomputation precedes durable ACK. | CT P00 |
| P00-AC-17 | Given mTLS issuer/Bridge/API mock, when ownership matrix reviewed, then external implementation responsibility and service owner approvals are explicit. | REV P00 |
| P00-AC-18 | Given MIT as-built, when Apache-2.0 plan reviewed, then license change is conditional on rights audit/NOTICE and release verification includes trusted signing. | LEGAL/SEC P00 |
| P00-AC-19 | Given approved lean single-Go-daemon scope and AD-01–03, when all 57 tickets and AC-001–055 are traced, then first-slice health, per-metric due/quality/aggregation and later network dependencies are owned, without plugin runtime or extra storage/service. | REV P00 |
| P00-AC-20 | Given all five design docs and backlog, when owners sign G00, then explicit evidence/sign-offs exist; docs approval alone does not authorize code or release. | REVIEW P00 |

## 3. Executable release-level AC-001–055

| AC ID | Required Given / When / Then and pass condition | Type / primary phase |
|---|---|---|
| AC-001 | Given baseline Linux fixtures, when parsers run, then valid CPU/memory/load/disk/net values and explicit partial errors are emitted, never silently replaced with valid zero. | UT P01 |
| AC-002 | Given Linux guest ticks and counter reset, when CPU usage is derived, then guest is not double-counted; invalid deltas are quality-flagged; range remains physically valid. | UT P01 |
| AC-003 | Given a fresh Linux host and new Agent package, when started, then only the daemon and its version/health/Hello contract are required; legacy CLI fixtures or migration are not tested. | IT P01 |
| AC-004 | Given SIGTERM/SIGINT of the new daemon, when shutdown runs, then in-flight SQLite transaction commits or rolls back, sockets cancel, process stops within agreed bound and no goroutine/orphan process remains. | IT P01/P10 |
| AC-005 | Given a fresh DB, when initialized, then the only logical Agent tables are `server_info`, `raw_samples`, `aggregates` with approved indexes/constraints. | IT P02 |
| AC-006 | Given SQLite busy/crash/WAL checkpoint stress, when reopened, then integrity is OK and there is no partially committed logical operation. | FT P02 |
| AC-007 | Given an empty DB path, interrupted initial bootstrap, or unknown incompatible schema version, when Agent starts, then creation is atomic or startup fails closed without corruption; no old-schema migration is attempted. | FT P02 |
| AC-008 | Given restart/same-host restore/clone, when identity loads, then authorized host preserves stable ID; cloned host cannot reuse registration without a distinct Agent ID and central-side quarantine/approval; no secrets disclose. | IT/SEC P02/P06 |
| AC-009 | Given one-hour deterministic clock and valid host, when one common 1Hz frame scheduler runs, then 3600 frames have genuine elapsed time while 1s CPU/Network/Memory/Disk-I/O, 5s Load (720) and 30s capacity (120) schedule their own **real** due measurements, and metadata only at startup/change; no invented measurements. | NT/UT P03 |
| AC-010 | Given 1s/5s/30s due patterns, missing/error/stale/not_due/counter_reset/unsupported cases, counters >2^53 and mount/interface churn, when sampled/serialized, then original metric times, precision, per-entity quality, fresh-zero versus unknown and TTL age are preserved with no fake repeated raw metric. | UT P03 |
| AC-011 | Given absent networks/Bridge/API, when multi-rate collection continues, then 1Hz raw frames contain only genuine due observations/errors, bounded Live cache cannot write duplicate old metrics to SQLite, all gaps and offline history pressure are observable. | IT/FT P03 |
| AC-012 | Given a 1-hour run and restart, when resumed, then no duplicate raw IDs, runaway timers or per-second SSH reconnects occur. | NT P03 |
| AC-013 | Given time xx:xx:00.000/59.999, when minute closes, then `[start,end)` window includes each valid raw ID exactly once. | UT P04 |
| AC-014 | Given CPU 40/60 valid with one actual 1s spike, Load 8/12 valid and capacity 2/2 valid in the same full minute, when rolled up, then each denominator and measured extrema/time are preserved; off-cadence frames are not counted missing and an unseen between-30s capacity spike is not invented. | UT P04 |
| AC-015 | Given crash before/during minute transaction, when reopened, then both final minute and `saved` changes are all-or-none. | FT P04 |
| AC-016 | Given retry after completed minute transaction, when rerun, then no duplicate minute record or mis-marked raw ID is created. | FT P04 |
| AC-017 | Given saved and unsaved raw rows, when cleanup runs or crashes, then only saved rows are deleted. | FT P04 |
| AC-018 | Given 15 finalized minutes with 1s/5s/30s metrics, stale cached view and clock/gap cases, when 900s rollup runs, then per-metric due/valid counts (normal planned 900/180/30), valid-duration weighted statistics, actual measured extrema/time, entity quality and missing coverage remain correct; cached values do not inflate history. | UT P05 |
| AC-019 | Given crash around 15m commit/cleanup, when retried, then one final record exists and no required minute is lost before commit. | FT P05 |
| AC-020 | Given pending final records inserted out-of-order, when queried, then oldest unACKed 900s period is returned first. | IT P05 |
| AC-021 | Given 61 virtual days including unACKed old data, when retention runs, then ACKed data follows approved expiry policy and unACKed data is never silently dropped. | FT P05 |
| AC-022 | Given measured reference cardinality, when 60-day storage simulation runs, then DB/WAL sizing and cleanup comply with approved budget with documented assumptions. | NT P05 |
| AC-023 | Given config with N=1/3/10, when loaded, then priority equals array index, no fixed two-server constraint, unsupported versions rejected. | UT P06 |
| AC-024 | Given bad/untrusted/expired/revoked cert or wrong server name, when dialled, then no operational mTLS channel forms and there is no plaintext fallback. | SEC P06 |
| AC-025 | Given authenticated but unauthorized Agent/channel, when Hello or send executes, then authorization is denied with typed non-secret diagnostic. | SEC/CT P06 |
| AC-026 | Given certificate/key rotation during service life, when rotated, then new trusted identity reconnects and old revoked identity cannot transmit. | SEC/IT P06 |
| AC-027 | Given incompatible major protocol and oversized payload, when negotiating, then fail closed/bounded with no unbounded allocation. | SEC/CT P06 |
| AC-028 | Given subscribed mock Bridge and due CPU every 1s, Load every 5s and capacity every 30s, when Live frames are relayed, then envelopes distinguish actual new measurements from not_due and optional old cached display with original measured_at/age/TTL/stale; no historical DB access occurs. | CT/E2E P07 |
| AC-029 | Given a permanently slow Bridge, when collection continues, then sample writes and rollups remain independent; live buffer stays bounded with explicit drops. | FT/NT P07 |
| AC-030 | Given network disconnection and re-establishment, when stream reconnects, then fresh live data resumes without fabricated historical backfill or per-tick SSH. | IT P07 |
| AC-031 | Given Persistence API down, when Live Bridge stays up, then live samples continue through Bridge without waiting for central ACK. | E2E P07 |
| AC-032 | Given raw or minute payload to the Persistence mock, when submitted, then protocol rejects it; only finalized 900s records qualify. | CT P08 |
| AC-033 | Given a 200 response without authenticated durable ACK, when handled, then `persisted_at` stays NULL. | FT/CT P08 |
| AC-034 | Given valid durable ACK for the same agent/period/ID/hash/domain, when verified, then only the matching final row gets `persisted_at` exactly once. | IT/SEC P08 |
| AC-035 | Given success at central commit but lost ACK, when resubmitted across restart, then exactly one central logical record exists. | FT/E2E P08 |
| AC-036 | Given wrong hash, unknown agent or forged/domain-mismatched ACK, when received, then state never advances to persisted. | SEC/CT P08 |
| AC-037 | Given throttling, timeout and out-of-order database rows, when replayed, then retries are bounded with jitter and oldest-first order preserved. | IT P08 |
| AC-038 | Given 1/30/61 days API outage or injected ENOSPC, when recovery runs, then Pending remains protected, live/sampling continues while storage capacity permits, and storage failure/loss is explicitly observable, never claimed lossless. | FT/NT P08 |
| AC-039 | Given N=3 mixed health/capabilities, when selecting per channel, then exactly one highest-priority eligible endpoint per channel is active. | IT P09 |
| AC-040 | Given primary outage and recovery, when failover/failback occurs, then configured backoff and hysteresis prevent connection flapping. | FT P09 |
| AC-041 | Given Live primary down but Persistence primary healthy (and reverse), when operating, then two channels may select different servers without interference. | E2E P09 |
| AC-042 | Given shared-domain endpoints, when ACK disappears and failover occurs, then deduplication stays stable across both paths. | E2E P09 |
| AC-043 | Given **independent** persistence domains, when a failover would imply false availability of Server A's data on B, then unsafe selection/ACK assumption is blocked and observable. | SEC/FT P09 |
| AC-044 | Given endpoint revocation/reorder under load, when config reloads, then revoked authority is not reused and resources are bounded. | SEC/IT P09 |
| AC-045 | Given systemd stop/restart/crash on disposable Linux, when restarted, then local SQLite remains valid, service enters healthy state and no orphan process stays. | NT/FT P10 |
| AC-046 | Given non-root runtime with malicious file permissions, when opening config/cert/DB, then protected paths are enforced and no secret is logged. | SEC P10 |
| AC-047 | Given clean installation and a new Desktop integration, when Agent starts and sends versioned Hello, then only the new contract is accepted and Desktop core SSH remains untouched; no legacy-client migration testing. | CT P10 |
| AC-048 | Given release artifacts, when checked independently, then signed provenance and checksums verify against trusted publisher; invalid signature fails. | SEC P10 |
| AC-049 | Given build targets amd64/arm64/armv7/armv6, when CI completes, then compilation succeeds and native runtime results/unsupported caveats are reported separately. | NT P10 |
| AC-050 | Given package install/upgrade/uninstall on supported OS, when operated as service, then files/identity/permissions behave as documented and no unsafe replacement occurs. | NT P10 |
| AC-051 | Given a clean Linux VM and only public docs, when operator follows onboarding/recovery guide, then can collect data and connect to contract mocks without undocumented steps. | E2E P10 |
| AC-052 | Given full crash/corrupt/disk-full/clock/ACK-loss, independent 1s/5s/30s due-scheduling and stale-cache suite, when replayed, then **INV-01–13** remain true or approved explicit degraded/error states are exposed; no unseen metric is asserted fresh. | FT P11 |
| AC-053 | Given N=3 multi-host rig and independent outages, when 1Hz Live multi-rate metric frames and finalized 900s Persistence transmit, then both protocols preserve per-metric timestamps/cadence/quality and ACK/domain rules, with no history through Bridge. | E2E P11 |
| AC-054 | Given native rigs and generated 60/61-day multi-rate sample/profile data, when benchmarked, then 1Hz frame jitter P99 <=500ms, peak RSS <=128MB, mean CPU <=5% of one core, actual 1s/5s/30s due rates and health accuracy, DB/WAL usage and replay >=2 final/s target (or explicit approved evidence-backed exception) are reported. | NT P11 |
| AC-055 | Given candidate release, when architecture, QA, security, licensing and external-interface owners review evidence, then all P0/P1 release blockers close and sign-off is explicit. | Review P11 |


## 4. Explicit failures and fixture inventory

| Scenario / fixture | Evidence / invariant |
|---|---|
| SIGKILL before/mid/after raw/minute/15m transaction; WAL checkpoint | SQLite resumes with no partially committed logical stage and no false saved/ACK; no corruption or explicit fail-closed |
| 40 valid/60 expected with one-second max spike | Minute and quarter preserve observed coverage, min/max/peak/time, elapsed-weighted mean and missing 20 |
| 120s grace, minute/quarter boundary, delayed raw, NTP forward/back, DST, reboot | UTC half-open assignment; late-after-freeze quality reported; no fabricated clock-corrected sample |
| Missing CPU, network parser truncation, unreadable mount, reset/wrap, hotplug, 1000 interfaces | Multi-rate due and not_due are distinct; partial errors explicit; zero not misused; resources bounded |
| SQLite BUSY, ENOSPC, disk permissions, corrupt DB, inconsistent WAL backup | no hidden wipe or automatic identity change; recovery documented |
| 1/30/61 days offline, restart mid-cleanup | unACKed final protected beyond 60d, ACKed expiry uses period-end, replay oldest-first |
| HTTP 200 absent durable ACK, wrong agent/period/hash/domain, unknown schema, spoofed or stale ACK | no `persisted_at` mutation |
| central commit succeeded / ACK lost / duplicate replay | one central logical row and matching subsequent durable ACK |
| Bridge blocked, disconnect, auth revoked, slow consumer | Live latest-only drops counted; sampler/Persistence independent |
| N=1/3/10 endpoint changes, failback flapping, domain split | one active per channel, correct array priority, Persistence domain safety |
| Cert wrong CA/name/SNI, expired/revoked, valid-cert wrong ACL, stolen enrollment token | fail closed and secrets redacted |
| Same-host restore vs cloned image | authorized identity preserved vs fresh independent enrollment |
| New Desktop integration and daemon | new protocol only, clean installation and unchanged Desktop core SSH |
| Package upgrade/downgrade, unsigned/tampered artifact, ARM native | version/schema safe, trusted verification mandatory, platform qualification explicit |

## 4A. Mandatory multi-cadence and freshness fixture suite

| Fixture | Testable assertion | Primary IDs |
|---|---|---|
| Deterministic 1h fake monotonic clock, no failures | 3600 actual frame opportunities, 1s metric due counts 3600, Load 720 and Filesystem capacity 120; static metadata only on startup/change; no unbounded worker timers | AC-009, AC-012 |
| Full 60s mixed-rate minute, CPU 40 valid with observed 1s spike, Load 8 valid, capacity 2 valid | Independent 40/60, 8/12 and 2/2 coverage; extrema/time from actual readings only; no 48 false missing Load slots | AC-014, AC-018 |
| 30s Filesystem checks with an injected capacity change between checks but not observed | No claim that hidden sub-30s spike was measured; accurate resolution and stale/age indication | AC-010, AC-014 |
| Last valid Load at t=0, subsequent not_due frames and later TTL expiry | No repeated raw Load; original measured_at stays t=0 in optional Live cache, status moves from within-TTL old to stale, no new sample counts | AC-010, AC-011, AC-028 |
| Due memory read error, actual zero network counter and unsupported disk_io | Typed error/null distinct from valid(0) and unsupported; per-metric quality survives 60s and 900s | AC-010, AC-018 |
| Counter wrap/reset, latency overrun, clock step, process restart/profile version change | No negative/false rate, unobserved due counts explicit, old profile's immutable final not rewritten; no extra raw frames | AC-009, AC-010, AC-018 |
| Slow Bridge and fast Disk I/O rate collector under 128MB budget | Live drops old events rather than backpressuring collector; Health exposes per-metric age, Collector/Storage/Persistence independently | AC-011, AC-029, AC-054 |
| Full N=3 Live+Persistence with mixed cadences and lost ACK | Live frame correctly distinguishes not_due/cache; final hashes metric-specific coverage; retry byte-identical, authorized ACK and one central record | AC-028, AC-034, AC-035, AC-053 |

The due/coverage fixtures above are **required acceptance**, not production test results. A PASS must supply a deterministic schedule phase, original source timestamps and weighted numerator/denominator verification in linked logs. A fresh 1s frame does **not** make the Load or capacity value fresh.

## 5. Phase gates and owners

| Gate | Required proof and owners |
|---|---|
| G00 | Five canonical design docs + this single AC matrix + updated implementation backlog; product/architecture + SQLite/data + protocol/security + external-interface + QA sign-offs; P00-AC-01–20 PASS with links. Owner approval of design text alone is not the same as executable evidence. |
| G01 | AC-001–004 with daemon-only collector extraction, signal handling and clean-install contract checks |
| G02 | AC-005–008 on 3-table schema, WAL/FULL, CGO-free qualifying drivers, corruption and clone |
| G03 | AC-009–012 with one controlled 1Hz frame scheduler, deterministic 1s/5s/30s due cadences, typed status, first-slice health and one-hour soak |
| G04 | AC-013–017 with UTC 120s grace, per-metric opportunity/quality-preserving 60s aggregate, atomic saved+minute and cleanup |
| G05 | AC-018–022 with 900s per-metric counts/valid-duration/measured peaks, 60-day ACKed cleanup and 61-day Pending survival |
| G06 | AC-023–027 with config N and mTLS/authz negative suite |
| G07 | AC-028–031 with live-only Bridge mocked subscriber and blocked consumer |
| G08 | AC-032–038 with durable central mock, hash/domain ACK and replay after loss |
| G09 | AC-039–044 with independent N endpoint failover, shared vs split domain |
| G10 | AC-045–051 with non-root systemd, signed package, rights audit, native architectures and clean installation |
| G11 | AC-052–055, all prior gates, external owner acceptance and signed product/security/QA release review |

**Sign-off ledger:** P00 Product/architecture owner = approved choice/design and docs update on 2026-10-18; SQLite/data owner = PENDING DDL execution review; PKI/Live/Persistence/Historical/Transible consumer owners = PENDING external contracts; QA = PENDING executable evidence; legal = PENDING copyright clearance. Do not mark G00 PASS or begin P01 merely because documents were committed.

## 6. NFR reference harness (DEC-08)

Reference load: pinned Linux kernel, CPU/arch, disk/FS type, interface/mount cardinality, **1s frame and independent 1s/5s/30s due policy, quality/freshness TTL, clock/profile churn, startup/change metadata**, 1-hour warm and steady run, 60/61-day accelerated DB/WAL fixture, network RTT, 1/30/61-day disconnection, failover and 1Hz P99. Report mean **process CPU seconds/wall seconds <=0.05 of one core**, peak **RSS <=128MB**, **P99 scheduler start deviation <=500ms** under qualified reference; engineering replay target >=2 final records/s against healthy local mock with owner-approved exception only after evidence; disk measured rather than assumed fixed quota. Native amd64/arm64/armv7/armv6 tests must be distinguished from cross-build.

## 7. Traceability and change control

P00-AC-01–20 -> AG-0001–0005; AC-001–004 -> AG-0101–0105; AC-005–008 -> AG-0201–0205; AC-009–012 -> AG-0301–0304; AC-013–017 -> AG-0401–0404; AC-018–022 -> AG-0501–0505; AC-023–027 -> AG-0601–0604; AC-028–031 -> AG-0701–0704; AC-032–038 -> AG-0801–0805; AC-039–044 -> AG-0901–0905; AC-045–051 -> AG-1001–1006; AC-052–055 -> AG-1101+ in the single backlog. Ticket-specific pass steps must map to these IDs, and missing coverage blocks the corresponding gate. Preserve IDs; update contract and backlog together on changes.

**Source inventory and constraints:** historical audit only covered CLI v1 and its CI packages; current main contains daemon v0.2.0; no actual new architecture integration or native four-arch qualification. historical Git tag `v0.1.0` protocol documentation remains historical AS-IS. Only new docs and existing backlog are authorized by this review; no code, live service, Desktop, license, release or production changes.

## 8. Operator-managed config and storage: additional design/implementation acceptance (2026-10-18)

These are **new proposed acceptance cases**; they are not PASSED and do not alter existing AC-001–055 or P00-AC-01–20 IDs. Allocate stable new IDs during backlog/G00 review, with prerequisites in the relevant P02/P06/P10/P11 gates.

| Case | Given / When / Then | Evidence |
|---|---|---|
| MGMT-01 | Desktop/operator connects using existing SSH/Ansible; when editing config or inspecting storage, then no new Agent inbound port, SSH implementation or management server exists, and unattended monitoring works with Desktop closed. | Integration/security |
| MGMT-02 | Given current valid config, when malformed, unknown critical field, unsafe limit, invalid ownership or unauthorized endpoint is staged, then the prior active config continues, error and revision divergence are reported, and no partial reload occurs. | Unit/fault |
| MGMT-03 | Given valid revisions, when edited atomically over SSH and reloaded or explicitly restarted, then applied versus desired values and required restart are reported; no secret-bearing diagnostic leaks. | Integration |
| MGMT-04 | Given CPU/RAM/filesystem thresholds and valid/stale/not_due/error/true-zero measurements, when Desktop evaluates alerts, then only eligible real measurements trigger/clear according to duration/hysteresis; closed-Desktop delivery remains an explicitly external Desktop contract. | Desktop contract fixture |
| MGMT-05 | Given desired Agent CPU/memory limits, when authorized SSH/Ansible applies systemd/cgroup policy, then effective limits are verified, non-root self-elevation is impossible, unsafe undersizing/restart/throttling is observable. | Native Linux/security |
| MGMT-06 | Given a DB with WAL, unsaved raw, saved raw, 60s, ACKed eligible and protected Pending finals, when inventory is queried, then each count/cutoff is correct and reported physical versus estimated space has non-misleading units. | SQL/native |
| MGMT-07 | Given safe cleanup dry-run followed by concurrent ACK/finalization/restart, when execution is authorized, then deletion predicates are rechecked transactionally, only lineage-complete/expired ACKed sources are removed, and protected Pending/unsaved raw remain untouched. | Fault/integration |
| MGMT-08 | Given disk pressure and concurrent 1Hz sampling, when bounded cleanup/checkpoint is attempted, then sampling/storage degradation is explicit, no silent Pending discard occurs and no immediate reclaimed-byte promise is made. | Fault/performance |
| MGMT-09 | Given arbitrary/unauthorized local invocation, when requesting config write, resource-limit change, destructive cleanup or SQL injection, then OS privilege checks and restricted command schema reject it with redacted audit output. | Security |

**Gate policy:** Document-only updates do not constitute implementation evidence or override blocked G00. Incorporate cases into the canonical ticket/AC mapping during the next approved gate review; until then MGMT cases are explicit additions awaiting ID assignment and external Desktop UX agreement.
