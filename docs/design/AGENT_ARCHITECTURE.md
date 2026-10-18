# Infrasible Monitoring Agent — Target Architecture

> **Implementation reconciliation (main, PR #11 / v0.2.0):** This document retains the owner-approved target and acceptance criteria; its historical source-audit claims about a CLI-only repository are no longer current. The code now contains an initial Linux daemon, SQLite three-table implementation, outbound mTLS clients and local storage CLI. This does **not** imply P00–P11 acceptance, central interface signoff, release security qualification, full metric quality/freshness coverage, enrollment/rotation, failover authorization or Desktop UI integration. Treat earlier baseline language below as archived **design-time assessment**, not current shipped behavior. The authoritative current command list, build and deployment steps are [README](../../README.md), [Installation](../INSTALLATION.md), [Configuration](../CONFIGURATION.md) and [Operations](../DAEMON_RUNBOOK.md). Keep outstanding design invariants intact.\n\n


> **Greenfield scope:** New clean-install daemon; no legacy consumers, migrations or compatibility gates. Canonical decision: [Product Requirements](AGENT_PRODUCT_REQUIREMENTS.md), section 2.

**Status:** approved owner architecture choices; detailed architecture baseline for G00 review; no runtime implementation authorized. **Source:** `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed`, current agent v0.1.0 legacy CLI. **Contracts:** [Product Requirements](AGENT_PRODUCT_REQUIREMENTS.md), [SQLite](AGENT_SQLITE_SCHEMA.md), [Protocol/Security](AGENT_PROTOCOL_AND_SECURITY.md), [Acceptance](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md).

## 1. System context, trust boundaries and ownership

```text
Monitored Linux host
+---------------------------------------------------------------------+
| infrasible-agent (one non-root systemd-supervised daemon process)   |
|  lifecycle/health + versioned endpoint config + local identity     |
|  monotonic 1Hz frame -> due metric collectors (1/5/30s)               |
|                     |                    |                          |
|        SQLite single controlled writer   +-- bounded Live buffer   |
|        3 application tables              |     WSS/mTLS client     |
|        raw frames(1s) -> per-metric minute(60s) -> final(900s)                        |
|                                |                                    |
|                     oldest pending immutable final                   |
|                                +-- HTTPS/mTLS Persistence client     |
|  independently selected and authorized N endpoints per channel      |
+----------------------+--------------------------+-------------------+
                       | WSS Live                 | HTTPS Persistence
             +---------v----------+     +---------v------------+
             | Stateless Live      |     | Persistence API       |
             | Bridge: ephemeral   |     | durable idempotent    |
             | subscriptions only  |     | PostgreSQL commit     |
             +---------+-----------+     +----------+-----------+
                       |                            |
                Live subscribed UI            Historical API
                                                    |
                                             Historical UI only
```

Trust boundaries: host collector reads local Linux metric sources (no application secrets); Agent SQLite/config/key files on local disk; separate outbound mTLS identities/channel ACLs; server-authorized Persistence domain, not inferred from DNS; Bridge **never** stores durable monitoring history. No Agent access to PostgreSQL and no direct Agent->Desktop Live stream.

The Agent repository may implement **mock boundary contracts/tests only**, never production Bridge/API/database services. Transible Desktop may provision through its **existing** SSH implementation; this repository never edits Desktop core SSH or vault. All operating transports are Agent-initiated, and no public inbound listener is required.

## 2. In-process components (keep intentionally small)

| Component | Responsibility | Forbidden coupling |
|---|---|---|
| Main daemon / Supervisor | Startup/shutdown, signals, identity/config readiness, resource health and worker lifetime | Legacy CLI compatibility or per-tick SSH |
| Linux collector | Small built-in Go metric modules; CPU/Network/Memory/Disk-I/O 1s, Load 5s, Filesystem capacity 30s, metadata on startup/change; genuine timestamps/units/status/quality and bounded entity cardinality | Network delivery, mutating host or unbounded plugin runtime |
| Monotonic scheduler | One 1000ms **frame** tick with deterministic due plan for metric families; measured actual elapsed, unique frame IDs, skip rather than fabricate overrun ticks | Separate unbounded timer/goroutine for each metric, re-stamping cached old readings |
| Controlled SQLite writer | Serialize all DB mutations; `WAL` + `synchronous=FULL`; transactions and bounded BUSY retry | Waiting on Bridge or central API while holding DB transaction |
| Rollup manager | Finalize minute after 120s grace; compute per-metric scheduled/valid/failed coverage and freshness-bounded averages; construct 15m from immutable minute sufficients | Treating every slow metric as if observed 60/900 times or rewriting final historical payload |
| Live worker | Persistently connect WSS; consume bounded latest-only sample buffer; quality/drop counters | Raw/history replay or disk queue |
| Persistence worker | Query oldest pending 900s row; HTTPS keep-alive, domain-bound idempotent ACK validation, atomic local marking | Sending 1s/60s or direct central DB access |
| Endpoint manager | Parse one ordered JSON config, channel-specific eligibility/authz, independent active target/retry/failover | Treat independently stored domain as shared |
| Health/maintenance | From first slice expose collector due/fail/last-valid/age, clock trust, SQLite durability, channel health and pending/disk/WAL; controlled cleanup | Conflating process-running with healthy monitoring, hiding stale metrics or deleting protected rows |

No additional Redis, JSONL, broker, microservice, queue DB, general plugin runtime, embedded AI/UI or fourth application table. No inbound public REST/listener and no server-configuration automation. Persistence Queue is a SQL predicate over `aggregates`. Delivery follows vertical, fault-tested increments within P01–P11 rather than partially completed features.

## 3. Source-grounded AS-IS versus TO-BE

The current `main.go` has `cpuSample/cpuUsage`, `memoryFrom`, `loadFrom`, `interfacesFrom`, `filesystems`, `snapshot`, stdout CLI `version/snapshot/stream --json`; `version.go` defines v0.1.0. It has no agent daemon, local SQLite, durable API, certificate enrollment, failover, or signed artifacts. `docs/PROTOCOL.md` is strictly the **legacy v1** stdout/SSH protocol. Preserve proven collector logic by test-driven extraction, but do **not** preserve removed public CLI commands in the new daemon. Version metadata must be available via approved registration/Hello/health contract rather than a legacy public `version --json` command. No v0.1.0 migration, coexistence or backward compatibility is required: new deployments start clean; old code remains reference material only.

Verified code corrections needed when implementation is separately approved: `cpuUsage` sums guest/guest_nice redundantly with Linux user/nice; partial network scanner/read failures and mount/statfs failures are silently omitted; snapshot waits internally one second instead of consuming one shared monotonic scheduler; raw counters are not delta/rates; stdout stream has no structured signal shutdown. Required classification: `main.go` REFACTOR, `version.go` REPLACE contract, parser tests KEEP/EXTEND, CLI tests REPLACE, packaging/CI EXTEND, v1 docs ARCHIVE AS-IS.

## 4. Time model and deterministic pipeline

- One actual **raw frame per completed nominal 1Hz tick**. Only due metric families produce new observations; a frame's `observed_at_utc` is the frame time, while each new metric has its **own `measured_at_utc`**, validity and interval. A boot-unique/random frame ID prevents collisions on restarts/backward clock steps. No invented fresh reading on `not_due` ticks.
- UTC periods are half-open `[start, end)`, 60s minutes and 900s quarters aligned to UTC epoch. Use one stored effective UTC attribution for each accepted raw sample; do not change it retroactively.
- Clock-adjustment handling: compare monotonic versus UTC elapsed; a large wall clock step (initial engineering alert threshold 2s) marks `CLOCK_UNTRUSTED`, pauses finalization/delivery of ambiguous windows and records explicit quality/gap evidence. Treat clock-stability release as an observable transition, never assign ambiguous samples silently.
- The minute `[12:00,12:01)` may finalize **no earlier than 12:03 UTC** (minute end + 120s) if clock trusted and every relevant admitted sample is durably processed. Frozen minutes are immutable; samples emerging after grace are not silently merged, and the late-drop/error count is exposed.
- A 900s quarter `[12:00,12:15)` cannot finalize until all its participating minute outcomes are immutable; with grace, no earlier than **12:17 UTC** in the ordinary case.
- **Coverage has two independent denominators:** base frame slots (nominal 60/900) and **metric-specific scheduled opportunities** (uninterrupted minute: 60 at 1s, 12 at 5s, 2 at 30s). Record due/attempted/valid/failed/missing per metric and entity, with original measurement time/quality. `not_due` ticks do not count missing. A 40/60 CPU fixture and 8/12 Load fixture remain distinct. A peak is the maximum **actually measured** (not an unobservable between-check spike). Gauge weighting must be freshness-bounded and transparently described.
- Counter metrics (Network/Disk I/O) use deltas over **their own real monotonic elapsed time**; reset/hotplug invalidates a rate interval. CPU/memory changes and mount/interface churn have stable entity keys and typed errors. Static metadata is collected at startup/change, not copied into every frame as fresh.
- Empty windows occurring **while Agent is running** may be represented as zero-coverage outcomes with missing-data flags; **fully offline host time must not be fabricated as collected monitoring data**. Recovery scans unfinished known windows with bounded work after restart.

```text
clock trust + sample admitted
    -> due metric collector observations (1s / 5s / 30s; genuine measured_at)
    -> 1Hz raw frame saved=0, containing only newly observed/failed due fields
    -> optional old-value cache for *Live display only*, separate age/freshness
    -> SQLite frame commit (durable first)
    -> 60s [start,end) + 120s grace, all admitted writes drained
    -> per-metric scheduled/attempted/valid/missing coverage + actual peak/last
    -> BEGIN IMMEDIATE; INSERT immutable minute; UPDATE exact source raw saved=1; COMMIT
    -> batched DELETE raw WHERE saved=1 (only)
    -> 15 immutable minute outcomes, including gaps/partial quality
    -> BEGIN IMMEDIATE; INSERT immutable 900s final incl source lineage/hash; COMMIT
    -> batched DELETE source minute rows only after final commit
    -> HTTPS submit oldest pending final (outside DB transaction)
    -> verify authenticated durable matching ACK/domain
    -> BEGIN IMMEDIATE; SET final.persisted_at once; COMMIT
    -> final row cleanup after 60d only if ACKed
```

**Crash invariant:** A committed minute and its source `saved` flags are all-or-none. A quarter is fully committed before deleting minute sources. `persisted_at` is never based on HTTP status alone. Restarts re-query SQL for pending finals; no persisted in-flight queue.

### 4.1. Due schedule, freshness and fan-out

**Owner-approved default metric policy:** CPU, Network, Memory/Swap, Disk I/O **1s**; Load **5s**; Filesystem capacity **30s**; static metadata **startup/change**. These are genuine collection rates within one common 1Hz frame scheduler, not different independent daemons. Keep one bounded collector execution path; slow reads are bounded by deadlines and cannot spawn unbounded parallel workers or delay database transactions indefinitely.

At each scheduled frame, compute the due set for each family using the **monotonic cadence profile** and log enough profile/phase/version information to reconstruct planned opportunities across recovery. For the metrics due:
1. Start a bounded read, record start/end monotonic duration and `measured_at_utc`, normalize units and entity IDs.
2. Classify each due attempt as `valid`, `missing`, `error`, `counter_reset` or `unsupported` with structured per-metric diagnostics; a real zero is `valid(0)`, not a missing sentinel.
3. Commit exactly one raw frame for the actual tick, with newly measured/failed **due** metric events in the row JSON; `not_due` is implicit from the versioned due plan or explicit quality marker, never a new value.
4. Independently offer the current frame to the bounded Live Worker. Optional in-memory last-known display state retains original time and `freshness` metadata, never a fresh measurement or SQLite history row.

The collector's `observation_status` and displayed `freshness` are **orthogonal**: a reading can be a previously valid cached observation within freshness TTL and yet **not be newly measured on this frame**. Freshness TTL starts with `2 × metric cadence + allowed jitter margin` (per-metric tunable, bounded and versioned). `stale` is a presentation/health state derived from original measurement age; it does not reenter raw or count in aggregation.

**Reference healthy full minute:** frame=60 due, CPU=60, Load=12, capacity=2; quarter=900/900/180/30. At startup/phase changes, gaps, tick overruns or clock uncertainty, per-metric expected/scheduled/actual attempts must be computed from documented cadence phase and known host uptime. Time with no running Agent is marked unobserved/gap, not synthetically populated. The global `expected_slots=60/900` schema still means *1Hz frame slots*, not every collector's opportunity count.

**Aggregation semantics:** per-metric observation count and error count are based on actual due opportunities. For gauges, only a explicitly defined observed or bounded freshness-valid interval may contribute duration-weighted estimates; label last-observation-held estimates as such and do not carry across stale or clock-untrusted gaps. Peak/min/max/last refer to actual measurements and timestamps; 30s filesystem collection cannot claim to have detected a 1-second unobserved filesystem spike. Counter rates use valid true delta/elapsed, preserve resets/wrap as quality. Quarter aggregation merges minute sufficient statistics **without multiplying slower observations by frame count**. Both Live JSON and final 900s envelope carry per-metric quality/measurement time/coverage suitable for their consumers.

**Health from first vertical slice:** record each metric's last due attempt, last valid time, age, misses/errors, unsupported state, scheduler lateness and raw-commit success; keep Live and Persistence status separate. A running process is not proof that every metric is healthy.


## 5. State machines and resource isolation

**Sampler:** `INIT -> FRAME_DUE -> METRIC_DUE_SELECTION -> COLLECTING_DUE_ONLY -> RAW_COMMIT -> FRAME_DUE`; on source error, commit partial/typed quality if storage works; an unchanged cached value must never masquerade as a newly observed raw value. On overrun skip missed ticks (account for absent opportunities) without unbounded catch-up; on DB failure report `STORAGE_DEGRADED` without claiming durability.

**Minute:** `OPEN -> GRACE -> FINALIZING -> FINAL`; forced stop before commit leaves `OPEN/GRACE` retriable, committed final immutable; late-after-freeze is counted/quarantined in health, never edits a final row.

**Quarter:** `WAIT_FOR_15_MINUTES -> FINALIZING -> FINAL_PENDING -> ACKED -> RETENTION_ELIGIBLE`. `IN_FLIGHT` is transient process memory, reconstructed by querying `persisted_at IS NULL` after crash. Never add a persistent queue table.

**Live:** `DISCONNECTED -> CONNECTING -> AUTHENTICATING/NEGOTIATING -> ACTIVE -> BACKOFF -> NEXT_ELIGIBLE`; only fresh outbound data; buffer engineering default 2 events, drop oldest on overflow, expose drop counter. Network send cannot hold SQLite transaction or stall sampler.

**Persistence:** independent connection/endpoint state machine, one in-flight request initial default; durable ACK is accepted only if agent, ID, period, schema, immutable payload hash and authorized persistence domain match. Classify transient error versus authz/schema/conflict fatal/quarantine; retention remains local while blocked.

**Failover:** per-channel candidate search over ordered config, at most one active eligible endpoint. Failover from A to B for Persistence requires **verified** same central durable/dedup domain; Live has no such restriction. Backoff initial engineering default randomized exponential 1–60s; higher-priority failback after >=90s steady successful health. The exact bounds remain adjustable with ADR and test evidence.

**Shutdown:** stop new ticks, bounded flush already committed work, cancel sockets, finish/rollback current SQLite transaction, close DB, exit supervised process. SIGKILL correctness relies on transactions rather than clean shutdown.

## 6. Resource, recovery and storage pressure

- An authenticated network call never holds a DB transaction. All queues bounded; avoid growing goroutines and sample copies with N endpoints. Endpoint probes limited; capacity verified with N=1,3,10.
- `STORAGE_DEGRADED` on ENOSPC/BUSY outside bounded retries; never mark uncommitted samples saved, never claim durable samples if write fails, **never delete unACKed final rows to recover space**, surface explicit lost/missing interval counter and capacity alerts. Live may remain available but should label history not guaranteed.
- Corrupt SQLite: fail closed for durable path, preserve damaged files, protect original identity/keys, alert and require authenticated/manual recovery. No automatic blank reset; restore with a transactionally consistent SQLite backup/snapshot only. An incompatible or unknown schema fails closed; no pre-launch schema upgrade/downgrade migration path is required.
- Same-host restore intentionally preserves `agent_id` after validated identity/authorization; cloned host requires fresh `agent_id`, enrollment/key, and central revocation or rejection of duplicated concurrent credentials. Hostname/machine-id alone cannot reliably detect all unauthorized clones.
- Clock drift/correction, unexpected reboot, interface/mount churn, partial permissions and restart mid-cleanup all have deterministic quality or recovery outcomes. Never convert an unknown measurement into a valid zero.
- Local final 15m: 60-day ACKed retention, indefinite pending until authenticated durable ACK subject to physical-disk reality. Pending age/size, oldest period and WAL size are mandatory health fields; after 61-day isolation, replay can resume safely with central owner contract for late-arrival hourly/daily recomputation.

## 7. Deployment / filesystem / boundary availability

Preferred systemd Linux path contract: config `/etc/infrasible-agent/config.json`, DB `/var/lib/infrasible-agent/agent.db`, protected identity materials under `/var/lib/infrasible-agent/`, journals via `journald`. Use a dedicated non-root service user and restrictive ownership/modes for private keys. Optional backup files are consistent snapshots, **not** new logical primary storage tables. No public TCP listener; all operational mTLS sessions originate from Agent.

Config changes: validate full replacement file (schema version, URL schemes, exact channel capability, trust material paths, duplicate IDs, authorities/domain claims), write temp+fsync+rename from authorized management action, atomically swap in-memory snapshot; rejected config leaves previous valid running configuration. Revoked endpoint: cancel live session and in-flight delivery before using new authorized endpoint.

**SLOs and hardware** are defined by `AGENT_PRODUCT_REQUIREMENTS.md`: 128 MB peak RSS, <=5% of one core mean CPU, P99 <=500ms schedule jitter, >=2 final/s engineering replay target requiring qualification; all target architectures need native acceptance.

## 8. Failure / threat review matrix

| Injection | Must remain true / recovery |
|---|---|
| Crash before/mid/after minute SQL commit | minute + saved flags atomic; restart idempotent |
| Crash before/mid/after 15m commit | immutable 900s once; source minute not prematurely deleted |
| Crash during WAL checkpoint, host power loss | SQLite integrity or explicit corruption health, no false ACK |
| Crash after central commit before ACK | retry identical stable ID/hash; central one logical row |
| Corrupt DB, invalid schema, incompatible rollback | fail closed; preserve state; operator-controlled recovery |
| Disk exhaustion during raw/rollup/maintenance | explicit storage degradation and observed gaps; unACKed finals retained |
| 1/30/61 days network outage | continued collection until local capacity exhausted; pending protected; oldest-first replay; late arrival central contract |
| Live blocked or slow | bounded drop counter; no SQLite/cadence stall |
| Persistence API unavailable but Live works | Live unaffected, 900s finals pending locally |
| Live unavailable but Persistence works | finals continue, Live health degraded |
| Expired, forged, revoked, wrong-host certificate | no operational authorized channel, no plaintext fallback |
| Agent identity reused by cloned host | deny/quarantine duplicate until distinct enrollment |
| N=3 reconnect/failback, two unrelated DB domains | per-channel one active; no unsafe Persistence domain crossing |
| Clock jumps, daylight-saving, unavailable host | UTC boundaries and quality evidence, no invented observations |
| 1000 mounts/interfaces, unreadable proc entries | bounded RSS, visible per-metric/entity partial errors |
| Config reload removes authorized endpoint | old transport promptly canceled; new config atomic |
| Legacy v1 consumer coexistence | NOT APPLICABLE: no users or legacy deployments; new Desktop integration targets only new Agent contracts |

## 9. Lifecycle/release boundaries and readiness

P00 requires five canonical docs and updated single backlog, approved external contracts and measurable acceptance. P01 corrects collector; P02 SQLite; P03 1Hz; P04 minute; P05 final retention; P06 registration/mTLS; P07 Live; P08 Persistence; P09 HA; P10 packaging/rollback/security; P11 performance/security/acceptance. Repository owner approval of design/docs is **not** proof that code or external APIs exist. Implement after a separate explicit authorization for P01; no production mutation or release under this document.

Open **external blocking agreements**: durable central ACK/domain and late-arrival compaction guarantees; Live Bridge subscriber/wire compatibility; enrollment issuer/revocation; legal rights for Apache-2.0. Evidence-based driver choice/native ARM support remain phase gates rather than guessed completions.

## 10. Operator management boundary: SSH/Ansible only (2026-10-18)

An authorized user provisions and manages Agent through their existing Desktop SSH or Ansible workflow. Desktop is the *operator interface*, not a runtime dependency. Agent stays a single non-root unattended daemon with only its existing outbound WSS Live and HTTPS Persistence transports. **No additional inbound management API, listener, SSH subsystem or agent polling loop** is introduced.

The sole versioned Agent configuration remains `/etc/infrasible-agent/config.json`. Operator tooling stages and atomically replaces the file with constrained permissions and safe ownership; reload validates the complete config before replacing the active immutable snapshot, and errors preserve the prior active snapshot with observable diagnostic status. Explicitly expose on-disk/active revisions and whether a restart is required. Never interpret invalid partially written data as valid defaults. Management filesystem edits must not be confused with changes to credentials/trust, which follow their existing protected-file rotation contract.

Separate three concepts: (1) host-metric alert thresholds for Desktop evaluation and display; (2) daemon resource budgets/desired limits, with privileged systemd/cgroup enforcement via existing SSH/Ansible and effective-status verification; (3) local disk/data retention and maintenance controls, which must not weaken pending-final guarantees. Host CPU/RAM/disk alert thresholds are unrelated to Agent CPU quota, RSS cap and storage budget. Threshold evaluation and UI delivery are external Desktop responsibilities. Any local Agent CLI added for schema validation/storage inspection/safe cleanup is a one-shot operator command, not a legacy stdout monitoring stream or an inbound service.

The storage layer owns read-only storage inventory and guarded bounded cleanup. The Desktop can display physical DB/WAL/SHM bytes, filesystem free, logical reclaimable estimates, ACKed-expired and unsaved/protected Pending counts plus oldest Pending age, then explicitly request safe cleanup over SSH. `DELETE` does not imply immediate disk-space reclamation; checkpoint/vacuum/maintenance must remain bounded and avoid blocking 1Hz sampling. UnACKed finals and unsaved raw are never selected by ordinary cleanup. Administrative actions require OS-level authorization, redacted audit diagnostics and tests for races with finalization/ACK/restart.
