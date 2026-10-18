# Infrasible Monitoring Agent — SQLite Contract and Data Lifecycle

> **Greenfield scope:** New clean-install daemon; no legacy consumers, migrations or compatibility gates. Canonical decision: [Product Requirements](AGENT_PRODUCT_REQUIREMENTS.md), section 2.

**Status:** engineering-selected design baseline approved for documentation; SQL shown is a **reviewable DDL draft**, not an executed or migrated schema. **Source baseline:** current v0.1.0 has no SQLite. **Applies:** DEC-03/04/06/10/11, INV-01/02/03/04/07. See [Architecture](AGENT_ARCHITECTURE.md) and [Acceptance](AGENT_ACCEPTANCE_AND_TEST_MATRIX.md).

## 1. Constraints

**Final owner-approved sampling refinement:** one actual 1 Hz raw **frame**, with independent due measurement cadences: CPU/Network/Memory/Swap/Disk I/O 1s; Load 5s; Filesystem capacity 30s; host metadata at startup/change. Original measurement times, quality, age and **per-metric** due/valid/failed counts survive one-minute and fifteen-minute rollups. This changes no logical table count.

One local SQLite database, exactly three logical **application** tables `server_info`, `raw_samples`, `aggregates`. No JSONL, Redis, broker, queue table or separate intermediate database. Internal SQLite structures such as `sqlite_sequence` do not constitute an Agent application table. One controlled writer, concurrent bounded readers, local filesystem, `WAL`, `synchronous=FULL`, explicit transactions and `PRAGMA user_version`. Pure Go driver selected after native target qualification; first benchmark candidate `modernc.org/sqlite`, alternative `ncruces/go-sqlite3` subject to measured CPU/RSS compatibility. No implied driver has passed tests yet.

## 2. Three-table DDL (fresh-install schema draft; initial version to be finalized)

The canonical future implementation must retain these semantics even if constrained SQL types or column names change after review. Any such change requires an ADR and contract-test update.

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = FULL;
-- Set PRAGMA user_version to the approved integer in the atomic fresh bootstrap; version is not finalized by this draft.

CREATE TABLE server_info (
    singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    agent_id TEXT NOT NULL UNIQUE,
    identity_epoch INTEGER NOT NULL DEFAULT 1 CHECK (identity_epoch > 0),
    created_at_ms INTEGER NOT NULL,
    host_metadata_json TEXT NOT NULL CHECK (json_valid(host_metadata_json)),
    last_finalized_minute_start_ms INTEGER,
    last_finalized_quarter_start_ms INTEGER
);

CREATE TABLE raw_samples (
    sample_id TEXT PRIMARY KEY,
    observed_at_ms INTEGER NOT NULL,
    effective_at_ms INTEGER NOT NULL,
    elapsed_ms INTEGER NOT NULL CHECK (elapsed_ms >= 0),
    metrics_json TEXT NOT NULL CHECK (json_valid(metrics_json)),
    quality_json TEXT NOT NULL CHECK (json_valid(quality_json)),
    saved INTEGER NOT NULL DEFAULT 0 CHECK (saved IN (0, 1))
);
CREATE INDEX idx_raw_time ON raw_samples(effective_at_ms, sample_id);
CREATE INDEX idx_raw_unsaved ON raw_samples(effective_at_ms, sample_id)
    WHERE saved = 0;
CREATE INDEX idx_raw_saved_cleanup ON raw_samples(effective_at_ms, sample_id)
    WHERE saved = 1;

CREATE TABLE aggregates (
    resolution_seconds INTEGER NOT NULL
        CHECK (resolution_seconds IN (60, 900)),
    period_start_ms INTEGER NOT NULL
        CHECK (period_start_ms >= 0),
    finalized_at_ms INTEGER NOT NULL,
    sample_count INTEGER NOT NULL CHECK (sample_count >= 0),
    -- Frame-level 1Hz slots ONLY: metric-specific denominators live in JSON.
    expected_slots INTEGER NOT NULL CHECK (expected_slots IN (60, 900)),
    observed_slots INTEGER NOT NULL CHECK (observed_slots >= 0),
    summary_json TEXT NOT NULL CHECK (json_valid(summary_json)),
    quality_json TEXT NOT NULL CHECK (json_valid(quality_json)),
    lineage_sha256 TEXT NOT NULL,
    record_id TEXT,
    payload_sha256 TEXT,
    wire_payload BLOB,
    persisted_at_ms INTEGER,
    PRIMARY KEY (resolution_seconds, period_start_ms),
    CHECK (period_start_ms % (resolution_seconds * 1000) = 0),
    CHECK (observed_slots <= expected_slots),
    CHECK (expected_slots = resolution_seconds),
    CHECK (
        (resolution_seconds = 60
          AND record_id IS NULL AND payload_sha256 IS NULL
          AND wire_payload IS NULL AND persisted_at_ms IS NULL)
        OR
        (resolution_seconds = 900
          AND record_id IS NOT NULL AND payload_sha256 IS NOT NULL
          AND wire_payload IS NOT NULL)
    )
);
CREATE UNIQUE INDEX idx_final_record_id ON aggregates(record_id)
    WHERE resolution_seconds = 900;
CREATE INDEX idx_pending_final_oldest
    ON aggregates(period_start_ms)
    WHERE resolution_seconds = 900 AND persisted_at_ms IS NULL;
CREATE INDEX idx_ack_final_expiry
    ON aggregates(period_start_ms)
    WHERE resolution_seconds = 900 AND persisted_at_ms IS NOT NULL;
```

**SQL review notes:**
- The exact JSON1 availability/CGO-free SQLite driver functions must be qualified; when JSON validation is unavailable, use application validation or a vetted compatible driver before final DDL sign-off. SQL source is not assumed running.
- `period_start_ms >= 0` limits historical windows before Unix epoch intentionally; target agent is modern Linux; assess explicit edge fixture.
- `raw_samples.saved` means aggregated in a **committed minute**, not persisted centrally.
- `aggregates.persisted_at_ms` exists only on **900s final**, and is written only after authenticated domain-matching durable ACK.
- `wire_payload` holds the exact immutable serialized final submission bytes **including** the `payload_sha256` field for byte-identical retries. To avoid a self-referential hash, `payload_sha256` is SHA-256 over the RFC 8785 (JCS) canonical UTF-8 JSON bytes of the immutable submission object **with the `payload_sha256` member omitted**. Reject non-canonical/NaN/Infinity inputs; the server independently recomputes the same digest. The transmitted `wire_payload` is not itself the hashed byte string. This duplicates summary data (storage cost must be measured) and avoids nondeterministic re-serialization on retries.
- `record_id` derives deterministically from `agent_id`, UTC period start and period length **without cert-rotation epoch or software version**. Same final logical record remains same ID after cert renewal; a different payload for same ID is an error, not a silent replacement.
- UTC fields as signed integer epoch milliseconds; use `[start_ms, end_ms)`. Unknown/missing values are JSON null/quality statuses, **not valid zero**.
- A raw row is one **1-second frame**, not one newly measured value for every metric. Its frame time differs from each actual due metric measurement time stored in JSON. Slow Load/Capacity readings are never duplicated on intervening frames.
- Aggregate sample_count/expected_slots/observed_slots refer only to **frame-level 1 Hz cadence**. Metric-specific expected/due/attempted/valid/failed/coverage/age and measured extrema/timestamps are stored under summary_json/quality_json, not falsely calculated as 60 or 900 opportunities for a 5s/30s collector. A JSON sampling-policy version and schedule phase must permit historical interpretation.
- Cached last-known values and their staleness are for Live/health display only and are **not additional raw samples**; a stale value never increases a 1m or 15m observed count.

A trigger to prohibit mutation of finalized records is optional defense-in-depth, not a fourth table. Immutable-final enforcement is mandatory at the application writer and SQLite tests. Because `persisted_at_ms` is the one allowed update to final rows, trigger rules (if used) must explicitly permit NULL -> authenticated timestamp only and reject changed final hash/payload.

## 3. Identity and one-host semantics

`server_info` has `singleton_id=1`, stable `agent_id`, metadata for the **monitored host** and minimal trusted watermarks. No N endpoint rows in SQLite. N endpoints and their ordered priority are in one versioned external JSON config. The primary `agent_id` is created using secure randomness at initial enrollment and must survive valid same-host restore/reinstall. A cloned host gets a distinct Agent identity and fresh enrollment (hostname and machine-id alone do not prove physical identity). Private keys never reside in this table.

Watermarks must only advance after corresponding immutable row transactions and be crash-safe in the same transaction. A gap of host-offline time must not be fabricated as a sequence of fully observed samples. If a watermark is redundant and unsafe to maintain, derive it from committed aggregates and omit in final reviewed schema.

## 4. Sampling and quality contract

- Scheduler runs one monotonic 1000ms **frame tick** and selects only due 1s/5s/30s metric families for bounded execution; actual elapsed time is recorded. Overrun skips ticks and reports gaps rather than spawning collectors or fabricating samples.
- One stable random/collision-resistant **frame** ID per actual accepted tick, independent of UTC; one raw row per frame with only genuine due readings and due failures. Every actual measurement carries original timestamp, source, unit, status and stable entity identity.
- Collect CPU, memory/swap, network and Disk I/O at 1s; Load at 5s; Filesystem capacity at 30s; metadata at startup/detected change. Typed quality distinguishes valid zero, missing scheduled due, error, counter_reset, unsupported, not_due and stale **derived old value** (not a new measurement).
- CPU percentages: deltas excluding already included `guest`/`guest_nice` from total tick accounting; reset, hotplug, negative/zero denominator produce quality flags.
- Memory/free, swap and Load are point-in-time gauges with original measurement timestamps. Weighted estimates may cover only explicit freshness-valid bounded intervals (clearly labeled held-value estimate), not stale or unobserved gaps. Measured min/max/last and valid-duration denominators remain explicit.
- Filesystem capacity is read at 30s per mount with original time/stable mount identity, removed/error/unsupported status; absent entity not treated as zero free bytes, and no 1s unseen capacity spike claimed.
- Wire/JSON numeric exactness: represent large `uint64` counters as decimal strings in JSON payloads (especially beyond IEEE-754 safe integer 2^53−1); validate range and unit. SQL timestamps/counts remain bounded signed integers. Canonical JSON hashing must be stable for large counters.
- Network and Disk I/O use actual monotonic counter deltas/rates (default 1s) with reset/wrap/hotplug invalid-rate epochs; accumulate delta and valid elapsed time, not absolute-counter averages or unweighted instant rate means.
- Spike/peak: retain max value and UTC first occurrence (tie-breaking deterministic) plus anomaly category/count; preserve per-entity data and high-cardinality caps.
- Clock anomalies: compare wall UTC vs monotonic elapsed; initial engineering anomaly threshold 2s; quarantine/mark ambiguous attribution and suspend unsafe finalization until trusted, no retroactive mutation of frozen periods.

### 4.1. JSON payload contract for independent metric schedules

**Raw frame JSON:** the scalar SQL columns identify a real frame. Its metrics_json contains a sampling_profile_version and only *new* observations for metric families actually due in this frame, with each entry holding measured_at_ms, source/unit, stable entity key (where applicable), value and typed observation_status. Its quality_json holds the due-families set, due errors, the scheduler phase/version, clock quality and gaps. A family not due at a given frame may be absent or explicitly listed as not_due, **never present with a repeated value and a new measurement timestamp**. A last-known cache used by Live remains bounded memory, not raw_samples history. For example a 12:00:01 frame may contain fresh CPU, Memory, Network and Disk-I/O but *no* new 5-second Load or 30-second Filesystem value.

**Aggregate JSON:** summary_json stores per metric/entity a *separate* record with sampling_profile_version, cadence_seconds, nominal_expected_opportunities, known_due_opportunities, attempts, valid_observations, invalid_or_failed_observations, explicit unobserved gaps, numerator/denominator coverage, last_measured_at_ms, measured min/max/last/peak_at_ms, and any sufficient weighted numerator plus valid_duration_ms. quality_json carries reset/error/unsupported reasons, stale-at-window-end/freshness indicators and clock quality. For a complete, uninterrupted 60-second window the denominators are CPU 60, Load 12, Filesystem capacity 2; a healthy 15-minute quarter gives CPU 900, Load 180 and capacity 30. If the process was offline, scheduling profile changed or UTC was untrusted, explicitly distinguish nominal opportunity, known due, and unobserved interval rather than inventing a collected sample.

**Status model:** valid, missing, error, counter_reset, unsupported and not_due are collection/due events; stale is derived from an earlier actual measurement's age when viewed, **not another collection event**. Store original measured_at_ms and a metric-specific freshness_ttl_ms, default 2 × cadence + approved jitter allowance. A previously valid cached value still within TTL is *not a fresh observation on a later frame*. JSON null and a typed missing/error are distinct from numeric zero.

**Per-metric computation examples (same minute):**

| Family | Scheduled opportunities | Actually valid | Coverage | Correct behavior |
|---|---:|---:|---|---|
| CPU 1s | 60 | 40 | 40/60 | Retain one observed 1s spike in measured max and peak time; missing 20 not zero |
| Load 5s | 12 | 8 | 8/12 | Do not label the 48 not_due frames as misses or duplicate old Load |
| Filesystem capacity 30s | 2 | 2 | 2/2 | Two actual measurements only; no claim to know any 1s capacity spike |
| Network counter 1s | 60 | 59 | 59/60 | One reset epoch invalidates rate, not a zero/negative traffic measurement |

These are **individual synthetic fixtures**, not a claim that each metric's failure falls on a different frame. The tests must pin consistent frame IDs, actual timestamps, profile phase, counter resets and duration arithmetic. Profile changes must be applied prospectively with a versioned phase marker; final immutable records cannot be recalculated with a later profile.

**1m/15m fidelity:** minute transaction includes only actually committed source-frame IDs and atomically marks those raw rows saved. The 15m transaction combines 15 finalized minute sufficient statistics, with per-metric opportunity counts and bounded gauge valid durations, preserving original measured peaks and gaps. Never multiply 5s or 30s observations by the 1Hz frame count. Central final payload includes the same per-metric quality/coverage so History can distinguish fresh, stale and partial information from truly idle values.

## 5. Minute finalization (atomic)

**Eligible** only after `period_end + 120 seconds`, trusted clock, and draining all admitted/raw writes for that window. `[12:00:00,12:01:00)` normally becomes final no earlier than 12:03 UTC. Actual commit may be later.

```text
minute_start = floor_utc(observed_period, 60)
assert now_trusted >= minute_start + 60s + 120s
assert all admitted writes for this period committed/known
BEGIN IMMEDIATE
  SELECT raw rows WHERE effective_at_ms >= start AND < end
  ORDER BY effective_at_ms, sample_id
  compute per-metric/instance scheduled/attempted/valid/error/missing counts,
    original measured times, valid-duration weights, observed extrema/peaks,
    freshness/quality, profile/phase and deterministic lineage
  INSERT immutable aggregates(resolution=60, period_start_ms=start,...)
    ON CONFLICT only if identical finalized lineage/summary; else fail/alert
  UPDATE raw_samples SET saved=1 WHERE sample_id IN (EXACT source IDs)
  advance committed minute watermark if retained
COMMIT
```

A minute with zero valid samples *during an observed running/eligible interval* may be represented as a zero-coverage/missing-quality outcome. Do not create a fake valid zero reading or fill completely offline-host periods as observed monitoring. Repeating after crash yields the identical minute or an explicit conflict; never mark a sample saved when minute commit fails.

**Late samples:** arrival after immutable freeze never changes final and is counted in quality/health as late rejected; unsaved raw must not be silently deleted merely due to age. Admission/late-rejection needs a testable boundary. If already durably inserted, it must be explicitly flagged and protected until an approved operator disposition; otherwise reject before committing normal raw with a non-secret observable gap/error counter. No silently lost late data.

## 6. Fifteen-minute finalization (atomic)

The UTC quarter `[12:00,12:15)` becomes eligible only after all fifteen minute outcomes are immutable or explicitly resolved missing, ordinarily no earlier than **12:17** because the last minute ended at 12:15 and has a 120-second grace.

```text
BEGIN IMMEDIATE
  read 15 finalized minute outcomes for quarter, validate completeness/lineage
  aggregate metric numerators and *valid elapsed denominators*
  merge valid extrema/peak time, last, per-entity and anomaly summaries
  construct stable 900s logical record, deterministic record_id,
    JCS_canonical_payload_without_hash, SHA256(canonical_payload), immutable wire_payload_with_hash
  INSERT aggregates(resolution=900,...,persisted_at_ms=NULL)
    ON CONFLICT only if byte/hash identical; else fail/alert
  advance committed quarter watermark if retained
COMMIT
-- only AFTER durable 900s commit:
DELETE 60s source aggregates for that quarter in bounded batches
```

Do not average minute averages naïvely. Merge each metric's valid-duration-weighted numerator/denominator (or adequate sufficient statistics) and independent due/attempted/valid counts, including Load 180 planned observations per uninterrupted quarter and Capacity 30, not 900. Cached/stale values never inflate counts. Only actual measured peaks are retained, with per-entity coverage and gaps.

## 7. Persistence and retention SQL

Pending is a view/predicate, **not a table**:

```sql
SELECT period_start_ms, record_id, payload_sha256, wire_payload
FROM aggregates
WHERE resolution_seconds = 900
  AND persisted_at_ms IS NULL
ORDER BY period_start_ms ASC
LIMIT 1;
```

Network send occurs outside writer transaction. After valid authenticated and authorized ACK with matching `agent_id`, `record_id`, period, hash, protocol and verified domain:

```sql
BEGIN IMMEDIATE;
UPDATE aggregates
SET persisted_at_ms = :ack_verified_at_ms
WHERE resolution_seconds = 900 AND record_id = :record_id
  AND payload_sha256 = :payload_sha256
  AND persisted_at_ms IS NULL;
COMMIT;
```

A zero affected-row result must be reconciled as already-ACKed or suspicious mismatch, never assumed success. `persisted_at_ms` is time of locally verified ACK, **not** the central commit timestamp; server commit timestamp may appear inside ACK and logs, not silently overload this column.

**Expiry anchor selected:** end of UTC final period + 60 days; acknowledged finals only become cleanup-eligible after both a valid ACK and that age. If ACK first arrives on day 61, it may become cleanup eligible after local ACK verification. All older **unACKed finals are retained** until valid ACK, even >60 days, unless disk physically fails (then explicit failure, never hidden data drop). Cleanup in bounded committed batches, and re-check `persisted_at_ms IS NOT NULL` in SQL DELETE predicate. Do not rely on count alone.

**Raw:** cleanup `WHERE saved=1` only, after minute transaction. **Minute:** cleanup only after quarter durable local creation. No short time-based deletion of unsaved raw.

## 8. Disk-full, WAL, fresh bootstrap and restore

Storage pressure is inherently finite. Maintain metrics of DB/WAL bytes, free bytes, raw and pending rows/age, estimated ingest/day and last durable write. Early warning thresholds start with measured per-host volume; if insufficient space or SQLite returns ENOSPC/BUSY beyond bounded retry, set `STORAGE_DEGRADED`, preserve unACKed data, never fake success, expose missed samples/gap and continue independent Live if possible. No fixed storage cap can promise 61+ days of data on arbitrary disks. WAL checkpoints must be bounded and not run with unbounded reader starvation; pressure thresholds must account for WAL growth.

**Integrity:** `PRAGMA integrity_check` on suspicion/restore; clean boot may use light checks, but corruption mandates fail closed and keep original for forensic copy. Never automatically delete/replace the valid identity. A consistent SQLite online backup or `VACUUM INTO` (with atomic staging/rename) is permitted for maintenance/controlled recovery; copying only main `.db` while WAL active is prohibited. Backup is **not** an independent queue or fourth logical table. Recovery requires operator authorization and evidence. Restore from same host keeps ID after central check; clone gets fresh ID/key/cert, with old credential invalidation handled by issuer.

**Greenfield bootstrap:** create the three application tables and indexes atomically on a fresh database, then set/check `PRAGMA user_version`. Reject unsupported, unexpected or partially initialized schema without modifying existing files. There is no v0.1.0 data/CLI migration, no pre-launch upgrade or downgrade implementation, and no migration backup requirement. Still require crash-safe normal writes, restart, integrity verification and authenticated same-host disaster recovery. Future released-version schema upgrades are a separate post-launch design task.

**Durability caveat:** WAL + FULL is SQLite's chosen durability policy but assumes honest filesystem and flush semantics; test kill -9, fsync fault and disposable VM power interruptions. An integrity failure is an explicit critical state, not a silent reset.

## 9. Index/query/size verification

Index effectiveness tested with `EXPLAIN QUERY PLAN` for (a) unsaved minute window, (b) pending oldest final, (c) ACKed expiry, (d) raw saved cleanup. Avoid full table scans each second; no network calls inside DB transactions.

60d × 24h × 4 final/h = **5,760 finals**. Payload-only volume is 22.5MiB @4KiB/final, 90MiB @16KiB, 360MiB @64KiB and 1,440MiB @256KiB. Measured SQLite/indexes/WAL, cardinality, staging and unACKed older rows add more. A reference dataset must report DB, WAL, peak RSS, CPU and checkpoint latency, including simulated 61-day disconnected replay.

## 10. Review criteria

- Schema inspection finds exactly three Agent application tables, valid constraints, prepared SQL, time-aligned primary keys and `PRAGMA user_version`; no fourth queue table.
- Failpoints before/mid/after raw, 60s and 900s transactions preserve atomic invariants and idempotent restart.
- One-minute CPU 40/60 with actual one-second spike, Load 8/12, Filesystem healthy 2/2, and a deliberately old cached value preserve distinct coverage and no stale inflation through 900s; no unobserved 30s capacity spike is claimed.
- Midnight/DST/NTP forward/backward/restart and 120s grace have deterministic, non-synthetic behavior; missed offline-host windows not fabricated.
- 61-day unACKed fixtures survive cleanup; ACKed expiry obeys UTC period end; ENOSPC exposes failure without deleting pending.
- Invalid hash/domain/ACK and wrong-resolution data do not update `persisted_at_ms`.
- SQLite driver builds CGO-free amd64/arm64/armv7/armv6, native target evidence and `FULL` power/crash/recovery results attached before G02.
