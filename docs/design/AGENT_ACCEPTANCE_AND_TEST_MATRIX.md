# Infrasible Monitoring Agent — Acceptance and Test Matrix

**Status:** source-grounded acceptance baseline approved for documentation; **NOT EXECUTED** for new architecture. Release requirements are not test results. Existing executable baseline covers only legacy v0.1.0 code/CI.
**Owner decisions:** DEC-01–DEC-11 approved in conversation 2026-10-18. **Repository reference:** `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed`.
**Source of truth for acceptance:** THIS file; `AGENT_IMPLEMENTATION_BACKLOG.md` is the single phased execution plan and refers to stable IDs here.

## 1. Test levels and proof format

`UT` unit, `IT` integration, `FT` fault injection, `CT` external contract/mock, `E2E` end-to-end with mock/safe disposable services, `NT` native soak/performance, `SEC` security, `REV` review, `LEGAL` license review. A PASS requires recorded environment/image/arch, exact revision, fixture version, command, expected and observed result, relevant CI run or evidence artifact, responsible reviewer, and any remaining limitation. Code and live central-service behavior are **not** claimed implemented just because their mock tests pass.

## 2. P00 design acceptance (must close before code)


| P00 AC | Given / When / Then | Type |
|---|---|---|
| P00-AC-01 | Given owner DEC-01–11, when checked against every canonical doc, then all decisions have identical outcomes and recorded approval. | REV P00 |
| P00-AC-02 | Given current baseline SHA and v0.1.0 code, when inventory is reviewed, then AS-IS is separated from all proposed daemon behavior. | REV P00 |
| P00-AC-03 | Given DEC-07, when auditing P01/P10/AC criteria, then no new daemon v1 CLI preservation is required and a versioned Desktop migration gate exists. | CT P00 |
| P00-AC-04 | Given proposed DDL, when schema inspected, then exactly three application tables and all required keys/constraints/indexes exist. | SQL P00 |
| P00-AC-05 | Given a crash before/mid/after 60s and 900s transactions, when state transitions are paper-walked, then saved/rollup/persisted predicates remain atomic and idempotent. | FT-REVIEW P00 |
| P00-AC-06 | Given UTC 120s grace/clock rollback/partial observations, when window timelines simulated, then immutable boundaries and no fabricated values hold. | UT-REVIEW P00 |
| P00-AC-07 | Given 61-day offline and ENOSPC, when retention reviewed, then unACKed rows are never silently deleted and lost new samples are reported honestly. | FT-REVIEW P00 |
| P00-AC-08 | Given slow/disconnected Bridge, when Live contract reviewed, then bounded drops, fresh-only reconnect and no history storage are explicit. | CT P00 |
| P00-AC-09 | Given HTTP 200 without valid durable ACK, when protocol reviewed, then persisted_at remains NULL and stable replay ID/hash are required. | CT P00 |
| P00-AC-10 | Given central commit-success/ACK-loss, when mock semantics reviewed, then only one logical central final and an authenticated domain-bound ACK can complete local state. | CT P00 |
| P00-AC-11 | Given N=1/3/10 with mixed permissions, when selection documented, then priority is array order, one active per channel and independent endpoint choice. | CT P00 |
| P00-AC-12 | Given two independent persistence domains, when failover documented, then Agent cannot treat shared config string or prior ACK as cross-domain proof. | SEC P00 |
| P00-AC-13 | Given cert expiration/revocation/clone, when trust flow documented, then fail closed, re-enroll and identity handling are explicit. | SEC P00 |
| P00-AC-14 | Given SQLite corruption/host restore, when operator workflow documented, then no auto-reset and same-host versus clone identity is unambiguous. | FT-REVIEW P00 |
| P00-AC-15 | Given performance DEC-08, when reference harness reviewed, then RSS/CPU/jitter definitions and native arch setup are reproducible. | NT-REVIEW P00 |
| P00-AC-16 | Given 30-day central 15m retention and 61-day Agent Pending, when external API contract reviewed, then idempotent late acceptance/hourly-daily recomputation precedes durable ACK. | CT P00 |
| P00-AC-17 | Given mTLS issuer/Bridge/API mock, when ownership matrix reviewed, then external implementation responsibility and service owner approvals are explicit. | REV P00 |
| P00-AC-18 | Given MIT as-built, when Apache-2.0 plan reviewed, then license change is conditional on rights audit/NOTICE and release verification includes trusted signing. | LEGAL/SEC P00 |
| P00-AC-19 | Given approved scope, when all 57 ticket dependencies and AC-001–055 are traced, then no mandatory behavior is unowned or silently assumed by code. | REV P00 |
| P00-AC-20 | Given all five design docs and backlog, when owners sign G00, then explicit evidence/sign-offs exist; docs approval alone does not authorize code or release. | REVIEW P00 |

## 3. Executable release-level AC-001–055

| AC ID | Required Given / When / Then and pass condition | Type / primary phase |
|---|---|---|
| AC-001 | Given baseline Linux fixtures, when parsers run, then valid CPU/memory/load/disk/net values and explicit partial errors are emitted, never silently replaced with valid zero. | UT P01 |
| AC-002 | Given Linux guest ticks and counter reset, when CPU usage is derived, then guest is not double-counted; invalid deltas are quality-flagged; range remains physically valid. | UT P01 |
| AC-003 | Given legacy v0.1.0 CLI consumer fixtures and proposed v2 daemon install, when Desktop migration compatibility is reviewed, then legacy binary is not silently replaced, migration gates and v2 version/health contract are explicit; new daemon has no public v1 stdout CLI. | CT P01/P10 |
| AC-004 | Given SIGTERM/SIGINT of the new daemon, when shutdown runs, then in-flight SQLite transaction commits or rolls back, sockets cancel, process stops within agreed bound and no goroutine/orphan process remains. | IT P01/P10 |
| AC-005 | Given a fresh DB, when initialized, then the only logical Agent tables are `server_info`, `raw_samples`, `aggregates` with approved indexes/constraints. | IT P02 |
| AC-006 | Given SQLite busy/crash/WAL checkpoint stress, when reopened, then integrity is OK and there is no partially committed logical operation. | FT P02 |
| AC-007 | Given upgrade/failed migration/downgrade attempts, when schema version rules execute, then upgrade is atomic and unsafe binary/schema mismatch fails closed. | FT P02 |
| AC-008 | Given restart/same-host restore/clone, when identity loads, then authorized host preserves stable ID; cloned host cannot reuse registration without a distinct Agent ID and central-side quarantine/approval; no secrets disclose. | IT/SEC P02/P06 |
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
| AC-038 | Given 1/30/61 days API outage or injected ENOSPC, when recovery runs, then Pending remains protected, live/sampling continues while storage capacity permits, and storage failure/loss is explicitly observable, never claimed lossless. | FT/NT P08 |
| AC-039 | Given N=3 mixed health/capabilities, when selecting per channel, then exactly one highest-priority eligible endpoint per channel is active. | IT P09 |
| AC-040 | Given primary outage and recovery, when failover/failback occurs, then configured backoff and hysteresis prevent connection flapping. | FT P09 |
| AC-041 | Given Live primary down but Persistence primary healthy (and reverse), when operating, then two channels may select different servers without interference. | E2E P09 |
| AC-042 | Given shared-domain endpoints, when ACK disappears and failover occurs, then deduplication stays stable across both paths. | E2E P09 |
| AC-043 | Given **independent** persistence domains, when a failover would imply false availability of Server A's data on B, then unsafe selection/ACK assumption is blocked and observable. | SEC/FT P09 |
| AC-044 | Given endpoint revocation/reorder under load, when config reloads, then revoked authority is not reused and resources are bounded. | SEC/IT P09 |
| AC-045 | Given systemd stop/restart/crash on disposable Linux, when restarted, then local SQLite remains valid, service enters healthy state and no orphan process stays. | NT/FT P10 |
| AC-046 | Given non-root runtime with malicious file permissions, when opening config/cert/DB, then protected paths are enforced and no secret is logged. | SEC P10 |
| AC-047 | Given an old Desktop v1 CLI consumer and a daemon-v2 upgrade/rollback, when migration is attempted, then old binary remains usable until versioned consumer migration is approved; incompatible binary/schema replacement fails closed; Desktop SSH core is untouched. | FT/CT P10 |
| AC-048 | Given release artifacts, when checked independently, then signed provenance and checksums verify against trusted publisher; invalid signature fails. | SEC P10 |
| AC-049 | Given build targets amd64/arm64/armv7/armv6, when CI completes, then compilation succeeds and native runtime results/unsupported caveats are reported separately. | NT P10 |
| AC-050 | Given package install/upgrade/uninstall on supported OS, when operated as service, then files/identity/permissions behave as documented and no unsafe replacement occurs. | NT P10 |
| AC-051 | Given a clean Linux VM and only public docs, when operator follows onboarding/recovery guide, then can collect data and connect to contract mocks without undocumented steps. | E2E P10 |
| AC-052 | Given full crash/corrupt/disk-full/clock/ACK-loss fault suite, when replayed, then INV-01–10 remain true or an approved explicit degraded/error state is exposed. | FT P11 |
| AC-053 | Given N=3 multi-host test rig, when both channels run with independent outages, then 1s Live and finalized 900s Persistence match their distinct approved APIs. | E2E P11 |
| AC-054 | Given approved native reference rigs and generated 60/61-day data, when benchmarked, then peak RSS <=128MB, mean CPU <=5% of one core, schedule-start jitter P99 <=500ms, disk/WAL usage reported and qualified replay >=2 final/s target or explicit reviewed exception. | NT P11 |
| AC-055 | Given candidate release, when architecture, QA, security, licensing and external-interface owners review evidence, then all P0/P1 release blockers close and sign-off is explicit. | Review P11 |


## 4. Explicit failures and fixture inventory

| Scenario / fixture | Evidence / invariant |
|---|---|
| SIGKILL before/mid/after raw/minute/15m transaction; WAL checkpoint | SQLite resumes with no partially committed logical stage and no false saved/ACK; no corruption or explicit fail-closed |
| 40 valid/60 expected with one-second max spike | Minute and quarter preserve observed coverage, min/max/peak/time, elapsed-weighted mean and missing 20 |
| 120s grace, minute/quarter boundary, delayed raw, NTP forward/back, DST, reboot | UTC half-open assignment; late-after-freeze quality reported; no fabricated clock-corrected sample |
| Missing CPU, network parser truncation, unreadable mount, reset/wrap, hotplug, 1000 interfaces | Partial errors explicit; zero not misused; resources bounded |
| SQLite BUSY, ENOSPC, disk permissions, corrupt DB, inconsistent WAL backup | no hidden wipe or automatic identity change; recovery documented |
| 1/30/61 days offline, restart mid-cleanup | unACKed final protected beyond 60d, ACKed expiry uses period-end, replay oldest-first |
| HTTP 200 absent durable ACK, wrong agent/period/hash/domain, unknown schema, spoofed or stale ACK | no `persisted_at` mutation |
| central commit succeeded / ACK lost / duplicate replay | one central logical row and matching subsequent durable ACK |
| Bridge blocked, disconnect, auth revoked, slow consumer | Live latest-only drops counted; sampler/Persistence independent |
| N=1/3/10 endpoint changes, failback flapping, domain split | one active per channel, correct array priority, Persistence domain safety |
| Cert wrong CA/name/SNI, expired/revoked, valid-cert wrong ACL, stolen enrollment token | fail closed and secrets redacted |
| Same-host restore vs cloned image | authorized identity preserved vs fresh independent enrollment |
| Legacy Desktop CLI and new daemon binary | no silent incompatible replacement; migration gate and core SSH protection |
| Package upgrade/downgrade, unsigned/tampered artifact, ARM native | version/schema safe, trusted verification mandatory, platform qualification explicit |

## 5. Phase gates and owners

| Gate | Required proof and owners |
|---|---|
| G00 | Five canonical design docs + this single AC matrix + updated implementation backlog; product/architecture + SQLite/data + protocol/security + external-interface + QA sign-offs; P00-AC-01–20 PASS with links. Owner approval of design text alone is not the same as executable evidence. |
| G01 | AC-001–004 with daemon-only collector extraction, signal handling and versioned Desktop v1 migration review |
| G02 | AC-005–008 on 3-table schema, WAL/FULL, CGO-free qualifying drivers, corruption and clone |
| G03 | AC-009–012 with controlled 1Hz sampler and one-hour soak |
| G04 | AC-013–017 with UTC 120s grace, atomic saved+minute and cleanup |
| G05 | AC-018–022 with complete 900s final, 60-day ACKed cleanup and 61-day Pending survival |
| G06 | AC-023–027 with config N and mTLS/authz negative suite |
| G07 | AC-028–031 with live-only Bridge mocked subscriber and blocked consumer |
| G08 | AC-032–038 with durable central mock, hash/domain ACK and replay after loss |
| G09 | AC-039–044 with independent N endpoint failover, shared vs split domain |
| G10 | AC-045–051 with non-root systemd, signed package, rights audit, native architectures and migration |
| G11 | AC-052–055, all prior gates, external owner acceptance and signed product/security/QA release review |

**Sign-off ledger:** P00 Product/architecture owner = approved choice/design and docs update on 2026-10-18; SQLite/data owner = PENDING DDL execution review; PKI/Live/Persistence/Historical/Transible consumer owners = PENDING external contracts; QA = PENDING executable evidence; legal = PENDING copyright clearance. Do not mark G00 PASS or begin P01 merely because documents were committed.

## 6. NFR reference harness (DEC-08)

Reference load: pinned Linux kernel, CPU/arch, disk/FS type, interface/mount cardinality, 1-hour warm and steady run, 60/61-day accelerated DB/WAL fixture, network RTT, 1/30/61-day disconnection, failover and 1Hz P99. Report mean **process CPU seconds/wall seconds <=0.05 of one core**, peak **RSS <=128MB**, **P99 scheduler start deviation <=500ms** under qualified reference; engineering replay target >=2 final records/s against healthy local mock with owner-approved exception only after evidence; disk measured rather than assumed fixed quota. Native amd64/arm64/armv7/armv6 tests must be distinguished from cross-build.

## 7. Traceability and change control

P00-AC-01–20 -> AG-0001–0005; AC-001–004 -> AG-0101–0105; AC-005–008 -> AG-0201–0205; AC-009–012 -> AG-0301–0304; AC-013–017 -> AG-0401–0404; AC-018–022 -> AG-0501–0505; AC-023–027 -> AG-0601–0604; AC-028–031 -> AG-0701–0704; AC-032–038 -> AG-0801–0805; AC-039–044 -> AG-0901–0905; AC-045–051 -> AG-1001–1006; AC-052–055 -> AG-1101+ in the single backlog. Ticket-specific pass steps must map to these IDs, and missing coverage blocks the corresponding gate. Preserve IDs; update contract and backlog together on changes.

**Source inventory and constraints:** current only runs CLI v1 and CI package scripts; no actual new architecture integration or native four-arch qualification. `docs/PROTOCOL.md` remains historical AS-IS. Only new docs and existing backlog are authorized by this review; no code, live service, Desktop, license, release or production changes.
