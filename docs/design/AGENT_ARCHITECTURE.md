# Infrasible Monitoring Agent — Target Architecture

**Status:** approved owner architecture choices; detailed architecture baseline for G00 review; no runtime implementation authorized. **Source:** `main@8b43c8a20d3a00fb030634c46d4677eaf52661ed`, current agent v0.1.0 legacy CLI. **Contracts:** [Product Requirements](AGENT_PRODUCT_REQUIREMENTS.md), [SQLite](AGENT_SQLITE_SCHEMA.md), [Protocol/Security](AGENT_PROTOCOL_AND_SECURITY.md), [Acceptance](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md).

## 1. System context, trust boundaries and ownership

```text
Monitored Linux host
+---------------------------------------------------------------------+
| infrasible-agent (one non-root systemd-supervised daemon process)   |
|  lifecycle/health + versioned endpoint config + local identity     |
|  monotonic 1Hz scheduler -> read-only Linux collector               |
|                     |                    |                          |
|        SQLite single controlled writer   +-- bounded Live buffer   |
|        3 application tables              |     WSS/mTLS client     |
|        raw(1s) -> minute(60s) -> final(900s)                        |
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
| Main daemon / Supervisor | Startup/shutdown, signals, identity/config readiness, resource health and worker lifetime | CLI v1 stdout stream or per-tick SSH |
| Linux collector | Testable parsers for CPU/mem/load/filesystems/network, rate/quality metadata and bounded entity cardinality | Network delivery or mutating host |
| Monotonic scheduler | Schedule 1000ms ticks, measure actual duration, skip rather than fabricate missed ticks, assign immutable sample ID | Unbounded queue/timer goroutine per tick |
| Controlled SQLite writer | Serialize all DB mutations; `WAL` + `synchronous=FULL`; transactions and bounded BUSY retry | Waiting on Bridge or central API while holding DB transaction |
| Rollup manager | Finalize minute after 120s grace; construct 15m only after source minute outcomes; stable hashes/lineage | Rewriting finalized/ACKed historical payload |
| Live worker | Persistently connect WSS; consume bounded latest-only sample buffer; quality/drop counters | Raw/history replay or disk queue |
| Persistence worker | Query oldest pending 900s row; HTTPS keep-alive, domain-bound idempotent ACK validation, atomic local marking | Sending 1s/60s or direct central DB access |
| Endpoint manager | Parse one ordered JSON config, channel-specific eligibility/authz, independent active target/retry/failover | Treat independently stored domain as shared |
| Health/maintenance | Evidence for lag, disk/WAL pressure, cert expiry, clock trust, corruption, rejected records and controlled cleanup | Secret disclosure, deleting unACKed final or unsaved raw |

No additional Redis, JSONL, broker, microservice, queue DB or fourth application table. Queue for Persistence is a SQL predicate over `aggregates`.

## 3. Source-grounded AS-IS versus TO-BE

The current `main.go` has `cpuSample/cpuUsage`, `memoryFrom`, `loadFrom`, `interfacesFrom`, `filesystems`, `snapshot`, stdout CLI `version/snapshot/stream --json`; `version.go` defines v0.1.0. It has no agent daemon, local SQLite, durable API, certificate enrollment, failover, or signed artifacts. `docs/PROTOCOL.md` is strictly the **legacy v1** stdout/SSH protocol. Preserve proven collector logic by test-driven extraction, but do **not** preserve removed public CLI commands in the new daemon. Version metadata must be available via approved registration/Hello/health contract rather than a legacy public `version --json` command. Preserve old v0.1.0 package for migration; prohibit silent overwrites while Desktop consumes v1.

Verified code corrections needed when implementation is separately approved: `cpuUsage` sums guest/guest_nice redundantly with Linux user/nice; partial network scanner/read failures and mount/statfs failures are silently omitted; snapshot waits internally one second instead of consuming one shared monotonic scheduler; raw counters are not delta/rates; stdout stream has no structured signal shutdown. Required classification: `main.go` REFACTOR, `version.go` REPLACE contract, parser tests KEEP/EXTEND, CLI tests REPLACE, packaging/CI EXTEND, v1 docs ARCHIVE AS-IS.

## 4. Time model and deterministic pipeline

- One actual observation per scheduled tick (nominal 1 Hz). Capture `observed_at_utc` and monotonic measured elapsed time; a boot-unique/random stable ID prevents collisions on restarts and backward wall-clock steps.
- UTC periods are half-open `[start, end)`, 60s minutes and 900s quarters aligned to UTC epoch. Use one stored effective UTC attribution for each accepted raw sample; do not change it retroactively.
- Clock-adjustment handling: compare monotonic versus UTC elapsed; a large wall clock step (initial engineering alert threshold 2s) marks `CLOCK_UNTRUSTED`, pauses finalization/delivery of ambiguous windows and records explicit quality/gap evidence. Treat clock-stability release as an observable transition, never assign ambiguous samples silently.
- The minute `[12:00,12:01)` may finalize **no earlier than 12:03 UTC** (minute end + 120s) if clock trusted and every relevant admitted sample is durably processed. Frozen minutes are immutable; samples emerging after grace are not silently merged, and the late-drop/error count is exposed.
- A 900s quarter `[12:00,12:15)` cannot finalize until all its participating minute outcomes are immutable; with grace, no earlier than **12:17 UTC** in the ordinary case.
- Per metric and per entity, coverage is measured: observed, valid, missing/invalid, expected slots (nominal 60/900), actual elapsed, quality flags. Zero is a measurement only if read/parse succeeded. A 40/60 fixture must remain 40/60. A spike is preserved as `max`/`peak_at` independent of average. Durations/weights for gauges must be defined per metric.
- Counter metrics require delta/time semantics; reset/hotplug invalidates the corresponding rate interval rather than producing negative spikes. For filesystem/interface churn use stable entity keys and explicit missing/unavailable quality.
- Empty windows occurring **while Agent is running** may be represented as zero-coverage outcomes with missing-data flags; **fully offline host time must not be fabricated as collected monitoring data**. Recovery scans unfinished known windows with bounded work after restart.

```text
clock trust + sample admitted
    -> raw_samples(saved=0) committed (1Hz; durable first)
    -> 60s [start,end) + 120s grace, all admitted writes drained
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

## 5. State machines and resource isolation

**Sampler:** `INIT -> SCHEDULED -> COLLECTING -> RAW_COMMIT -> SCHEDULED`; on read error, commit partial/quality if storage works; on scheduler overrun skip missing ticks with gap evidence; on DB error enter STORAGE_DEGRADED without reporting durable success.

**Minute:** `OPEN -> GRACE -> FINALIZING -> FINAL`; forced stop before commit leaves `OPEN/GRACE` retriable, committed final immutable; late-after-freeze is counted/quarantined in health, never edits a final row.

**Quarter:** `WAIT_FOR_15_MINUTES -> FINALIZING -> FINAL_PENDING -> ACKED -> RETENTION_ELIGIBLE`. `IN_FLIGHT` is transient process memory, reconstructed by querying `persisted_at IS NULL` after crash. Never add a persistent queue table.

**Live:** `DISCONNECTED -> CONNECTING -> AUTHENTICATING/NEGOTIATING -> ACTIVE -> BACKOFF -> NEXT_ELIGIBLE`; only fresh outbound data; buffer engineering default 2 events, drop oldest on overflow, expose drop counter. Network send cannot hold SQLite transaction or stall sampler.

**Persistence:** independent connection/endpoint state machine, one in-flight request initial default; durable ACK is accepted only if agent, ID, period, schema, immutable payload hash and authorized persistence domain match. Classify transient error versus authz/schema/conflict fatal/quarantine; retention remains local while blocked.

**Failover:** per-channel candidate search over ordered config, at most one active eligible endpoint. Failover from A to B for Persistence requires **verified** same central durable/dedup domain; Live has no such restriction. Backoff initial engineering default randomized exponential 1–60s; higher-priority failback after >=90s steady successful health. The exact bounds remain adjustable with ADR and test evidence.

**Shutdown:** stop new ticks, bounded flush already committed work, cancel sockets, finish/rollback current SQLite transaction, close DB, exit supervised process. SIGKILL correctness relies on transactions rather than clean shutdown.

## 6. Resource, recovery and storage pressure

- An authenticated network call never holds a DB transaction. All queues bounded; avoid growing goroutines and sample copies with N endpoints. Endpoint probes limited; capacity verified with N=1,3,10.
- `STORAGE_DEGRADED` on ENOSPC/BUSY outside bounded retries; never mark uncommitted samples saved, never claim durable samples if write fails, **never delete unACKed final rows to recover space**, surface explicit lost/missing interval counter and capacity alerts. Live may remain available but should label history not guaranteed.
- Corrupt SQLite: fail closed for durable path, preserve damaged files, protect original identity/keys, alert and require authenticated/manual recovery. No automatic blank reset; restore with a transactionally consistent SQLite backup/snapshot only. Downgrade to binary unable to read current schema fails closed.
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
| Old v1 Desktop installs daemon binary in place | blocked by versioned migration gate, no implicit CLI compatibility |

## 9. Lifecycle/release boundaries and readiness

P00 requires five canonical docs and updated single backlog, approved external contracts and measurable acceptance. P01 corrects collector; P02 SQLite; P03 1Hz; P04 minute; P05 final retention; P06 registration/mTLS; P07 Live; P08 Persistence; P09 HA; P10 packaging/rollback/security; P11 performance/security/acceptance. Repository owner approval of design/docs is **not** proof that code or external APIs exist. Implement after a separate explicit authorization for P01; no production mutation or release under this document.

Open **external blocking agreements**: durable central ACK/domain and late-arrival compaction guarantees; Live Bridge subscriber/wire compatibility; enrollment issuer/revocation; Desktop v1 consumer migration (without core SSH rewrite); legal rights for Apache-2.0. Evidence-based driver choice/native ARM support remain phase gates rather than guessed completions.
