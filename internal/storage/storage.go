package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gemvc/infrasible-agent/internal/collector"
	_ "modernc.org/sqlite"
)

const Grace = 120 * time.Second
const Retention = 60 * 24 * time.Hour

// Store contains the only durable Agent state. SQLite serializes writes across
// the daemon and any explicitly authorized offline/operator maintenance CLI.
type Store struct {
	db   *sql.DB
	path string
	id   string
}
type Statistic struct {
	Count    int64   `json:"count"`
	Due      int64   `json:"due"`
	Failed   int64   `json:"failed"`
	Sum      float64 `json:"sum"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Last     float64 `json:"last"`
	PeakAtMS int64   `json:"peak_at_ms"`
	LastAtMS int64   `json:"last_at_ms"`
}
type Final struct {
	AgentID           string               `json:"agent_id"`
	SchemaVersion     int                  `json:"schema_version"`
	PeriodStartMS     int64                `json:"period_start_ms"`
	ResolutionSeconds int                  `json:"resolution_seconds"`
	RecordID          string               `json:"record_id"`
	SampleCount       int64                `json:"sample_count"`
	ExpectedSlots     int64                `json:"expected_slots"`
	ObservedSlots     int64                `json:"observed_slots"`
	Summary           map[string]Statistic `json:"summary"`
	PayloadSHA256     string               `json:"payload_sha256,omitempty"`
}
type Pending struct {
	RecordID      string
	PeriodStartMS int64
	Hash          string
	Payload       []byte
}
type StorageStatus struct {
	AgentID              string `json:"agent_id"`
	DBBytes              int64  `json:"db_bytes"`
	WALBytes             int64  `json:"wal_bytes"`
	SHMBytes             int64  `json:"shm_bytes"`
	FilesystemFreeBytes  uint64 `json:"filesystem_free_bytes"`
	FilesystemTotalBytes uint64 `json:"filesystem_total_bytes"`
	SQLiteFreePageBytes  int64  `json:"sqlite_free_page_bytes"`
	SQLiteAutoVacuumMode int64  `json:"sqlite_auto_vacuum_mode"`
	LateRaw              int64  `json:"late_raw"`
	RawUnsaved           int64  `json:"raw_unsaved"`
	RawSaved             int64  `json:"raw_saved"`
	Minutes              int64  `json:"minutes"`
	Finals               int64  `json:"finals"`
	Pending              int64  `json:"pending"`
	OldestPendingMS      *int64 `json:"oldest_pending_ms,omitempty"`
	EligibleFinals       int64  `json:"eligible_finals"`
	RetentionCutoffMS    int64  `json:"retention_cutoff_ms"`
}
type CleanupResult struct {
	DryRun  bool  `json:"dry_run"`
	Raw     int64 `json:"raw"`
	Minutes int64 `json:"minutes"`
	Finals  int64 `json:"finals"`
	// SQLite DELETE does not guarantee immediate disk-space recovery.
	DBBytesBefore int64 `json:"db_bytes_before"`
	DBBytesAfter  int64 `json:"db_bytes_after"`
}

const schema = `
CREATE TABLE IF NOT EXISTS server_info (
 singleton_id INTEGER PRIMARY KEY CHECK(singleton_id=1),
 agent_id TEXT NOT NULL UNIQUE,
 identity_epoch INTEGER NOT NULL DEFAULT 1 CHECK(identity_epoch>0),
 created_at_ms INTEGER NOT NULL,
 host_metadata_json TEXT NOT NULL CHECK(json_valid(host_metadata_json)),
 last_raw_at_ms INTEGER,
 last_ack_at_ms INTEGER,
 last_finalized_minute_start_ms INTEGER,
 last_finalized_quarter_start_ms INTEGER
);
CREATE TABLE IF NOT EXISTS raw_samples (
 sample_id TEXT PRIMARY KEY,
 observed_at_ms INTEGER NOT NULL,
 effective_at_ms INTEGER NOT NULL,
 elapsed_ms INTEGER NOT NULL CHECK(elapsed_ms>=0),
 metrics_json TEXT NOT NULL CHECK(json_valid(metrics_json)),
 quality_json TEXT NOT NULL CHECK(json_valid(quality_json)),
 saved INTEGER NOT NULL DEFAULT 0 CHECK(saved IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_raw_time ON raw_samples(effective_at_ms,sample_id);
CREATE INDEX IF NOT EXISTS idx_raw_unsaved ON raw_samples(effective_at_ms,sample_id) WHERE saved=0;
CREATE INDEX IF NOT EXISTS idx_raw_saved_cleanup ON raw_samples(effective_at_ms,sample_id) WHERE saved=1;
CREATE TABLE IF NOT EXISTS aggregates (
 resolution_seconds INTEGER NOT NULL CHECK(resolution_seconds IN (60,900)),
 period_start_ms INTEGER NOT NULL CHECK(period_start_ms>=0),
 finalized_at_ms INTEGER NOT NULL,
 sample_count INTEGER NOT NULL CHECK(sample_count>=0),
 expected_slots INTEGER NOT NULL CHECK(expected_slots IN (60,900)),
 observed_slots INTEGER NOT NULL CHECK(observed_slots BETWEEN 0 AND expected_slots),
 summary_json TEXT NOT NULL CHECK(json_valid(summary_json)),
 quality_json TEXT NOT NULL CHECK(json_valid(quality_json)),
 lineage_sha256 TEXT NOT NULL,
 record_id TEXT,
 payload_sha256 TEXT,
 wire_payload BLOB,
 persisted_at_ms INTEGER,
 PRIMARY KEY(resolution_seconds,period_start_ms),
 CHECK(period_start_ms % (resolution_seconds*1000)=0),
 CHECK(expected_slots=resolution_seconds),
 CHECK ((resolution_seconds=60 AND record_id IS NULL AND payload_sha256 IS NULL AND wire_payload IS NULL AND persisted_at_ms IS NULL) OR
        (resolution_seconds=900 AND record_id IS NOT NULL AND payload_sha256 IS NOT NULL AND wire_payload IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_final_record_id ON aggregates(record_id) WHERE resolution_seconds=900;
CREATE INDEX IF NOT EXISTS idx_pending_final_oldest ON aggregates(period_start_ms) WHERE resolution_seconds=900 AND persisted_at_ms IS NULL;
CREATE INDEX IF NOT EXISTS idx_ack_final_expiry ON aggregates(period_start_ms) WHERE resolution_seconds=900 AND persisted_at_ms IS NOT NULL;
`

// OpenExisting prevents a read-only operator command from implicitly creating a
// new Agent identity and database when the configured path is wrong.
func OpenExisting(path string) (*Store, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("agent storage must be an existing regular non-symlink file")
	}
	return Open(path)
}

func Open(path string) (*Store, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, q := range []string{"PRAGMA auto_vacuum=INCREMENTAL", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, e = db.ExecContext(ctx, q); e != nil {
			db.Close()
			return nil, e
		}
	}
	var version int
	if e = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		db.Close()
		return nil, e
	}
	if version != 0 && version != 1 {
		db.Close()
		return nil, fmt.Errorf("unsupported database user_version=%d", version)
	}
	if _, e = db.ExecContext(ctx, schema); e != nil {
		db.Close()
		return nil, e
	}
	if _, e = db.ExecContext(ctx, "PRAGMA user_version=1"); e != nil {
		db.Close()
		return nil, e
	}
	var id string
	err := db.QueryRowContext(ctx, "SELECT agent_id FROM server_info WHERE singleton_id=1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		raw := make([]byte, 16)
		if _, e = rand.Read(raw); e != nil {
			db.Close()
			return nil, e
		}
		id = hex.EncodeToString(raw)
		_, e = db.ExecContext(ctx, `INSERT INTO server_info(singleton_id,agent_id,created_at_ms,host_metadata_json) VALUES(1,?,?,?)`, id, time.Now().UTC().UnixMilli(), "{}")
		if e != nil {
			db.Close()
			return nil, e
		}
	} else if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path, id: id}, nil
}
func (s *Store) Close() error    { return s.db.Close() }
func (s *Store) AgentID() string { return s.id }
func (s *Store) Save(ctx context.Context, f collector.Frame) error {
	raw := make([]byte, 16)
	if _, e := rand.Read(raw); e != nil {
		return e
	}
	data, e := json.Marshal(f.Metrics)
	if e != nil {
		return e
	}
	if f.ObservedAtMS <= 0 {
		return errors.New("missing observed timestamp")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO raw_samples(sample_id,observed_at_ms,effective_at_ms,elapsed_ms,metrics_json,quality_json)
 VALUES(?,?,?,?,?,?)`, hex.EncodeToString(raw), f.ObservedAtMS, f.ObservedAtMS, f.ElapsedMS, string(data), `{"policy":"1s/5s/30s"}`)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE server_info SET last_raw_at_ms=CASE WHEN last_raw_at_ms IS NULL OR last_raw_at_ms<? THEN ? ELSE last_raw_at_ms END WHERE singleton_id=1`, f.ObservedAtMS, f.ObservedAtMS)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func aligned(ms int64, interval int64) int64 { return ms - ms%interval }
func digest(b []byte) string                 { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func clone(a map[string]Statistic) map[string]Statistic {
	b := make(map[string]Statistic, len(a))
	for k, v := range a {
		b[k] = v
	}
	return b
}
func merge(a *Statistic, b Statistic) {
	if b.Count > 0 {
		if a.Count == 0 || b.Min < a.Min {
			a.Min = b.Min
		}
		if a.Count == 0 || b.Max > a.Max {
			a.Max = b.Max
			a.PeakAtMS = b.PeakAtMS
		}
		if b.LastAtMS >= a.LastAtMS {
			a.Last = b.Last
			a.LastAtMS = b.LastAtMS
		}
	}
	a.Count += b.Count
	a.Due += b.Due
	a.Failed += b.Failed
	a.Sum += b.Sum
}
func record(summary map[string]Statistic, key string, v float64, at int64) {
	st := summary[key]
	merge(&st, Statistic{Count: 1, Due: 1, Sum: v, Min: v, Max: v, Last: v, PeakAtMS: at, LastAtMS: at})
	summary[key] = st
}
func flatten(summary map[string]Statistic, key string, v any, at int64) {
	switch x := v.(type) {
	case float64:
		record(summary, key, x, at)
	case map[string]any:
		for k, v := range x {
			flatten(summary, key+"."+k, v, at)
		}
	}
}
func fromRaw(rows *sql.Rows) (map[string]Statistic, int64, error) {
	result := map[string]Statistic{}
	var frames int64
	for rows.Next() {
		var raw string
		var at int64
		if e := rows.Scan(&raw, &at); e != nil {
			return nil, 0, e
		}
		frames++
		var metrics map[string]collector.Observation
		if e := json.Unmarshal([]byte(raw), &metrics); e != nil {
			return nil, 0, e
		}
		for name, o := range metrics {
			st := result[name]
			st.Due++
			if o.Status != "valid" {
				st.Failed++
			}
			result[name] = st
			if o.Status == "valid" {
				flatten(result, name, o.Value, o.MeasuredAtMS)
			}
		}
	}
	return result, frames, rows.Err()
}
func (s *Store) Finalize(ctx context.Context, now time.Time) error {
	cutoff := now.UTC().Add(-Grace).UnixMilli()
	// Find only completed minute buckets containing unsaved observations.
	rows, e := s.db.QueryContext(ctx, `SELECT DISTINCT effective_at_ms/60000*60000 FROM raw_samples
 WHERE saved=0 AND effective_at_ms/60000*60000+60000<=?
 AND NOT EXISTS (SELECT 1 FROM aggregates a WHERE a.resolution_seconds=60 AND a.period_start_ms=raw_samples.effective_at_ms/60000*60000)
 ORDER BY 1 LIMIT 128`, cutoff)
	if e != nil {
		return e
	}
	var minutes []int64
	for rows.Next() {
		var m int64
		if e = rows.Scan(&m); e != nil {
			break
		}
		minutes = append(minutes, m)
	}
	if closeErr := rows.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	for _, m := range minutes {
		if e = s.minute(ctx, m, now.UTC().UnixMilli()); e != nil {
			return e
		}
	}
	// Finalize only 900s windows whose end has passed the same grace.
	rows, e = s.db.QueryContext(ctx, `SELECT DISTINCT period_start_ms/900000*900000 FROM aggregates
 WHERE resolution_seconds=60 AND period_start_ms/900000*900000+900000<=? ORDER BY 1 LIMIT 32`, cutoff)
	if e != nil {
		return e
	}
	var quarters []int64
	for rows.Next() {
		var q int64
		if e = rows.Scan(&q); e != nil {
			break
		}
		quarters = append(quarters, q)
	}
	if closeErr := rows.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	for _, q := range quarters {
		if e = s.quarter(ctx, q, now.UTC().UnixMilli()); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) minute(ctx context.Context, start, now int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var exists int
	e = tx.QueryRowContext(ctx, "SELECT 1 FROM aggregates WHERE resolution_seconds=60 AND period_start_ms=?", start).Scan(&exists)
	if e == nil {
		return fmt.Errorf("late unsaved raw in immutable minute %d (operator review required)", start)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT metrics_json,observed_at_ms FROM raw_samples WHERE saved=0 AND effective_at_ms>=? AND effective_at_ms<? ORDER BY effective_at_ms`, start, start+60000)
	if e != nil {
		return e
	}
	summary, frames, e := fromRaw(rows)
	rows.Close()
	if e != nil {
		return e
	}
	if frames == 0 {
		return tx.Commit()
	}
	payload, e := json.Marshal(summary)
	if e != nil {
		return e
	}
	lin := digest(payload)
	_, e = tx.ExecContext(ctx, `INSERT INTO aggregates(resolution_seconds,period_start_ms,finalized_at_ms,sample_count,expected_slots,observed_slots,summary_json,quality_json,lineage_sha256) VALUES(60,?,?,?,60,?,?,?,?)`, start, now, frames, frames, string(payload), `{"policy":"1s/5s/30s"}`, lin)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE raw_samples SET saved=1 WHERE saved=0 AND effective_at_ms>=? AND effective_at_ms<?`, start, start+60000)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE server_info SET last_finalized_minute_start_ms=? WHERE singleton_id=1 AND (last_finalized_minute_start_ms IS NULL OR last_finalized_minute_start_ms<?)`, start, start)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) quarter(ctx context.Context, start, now int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var existing int
	e = tx.QueryRowContext(ctx, "SELECT 1 FROM aggregates WHERE resolution_seconds=900 AND period_start_ms=?", start).Scan(&existing)
	if e == nil {
		return tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT summary_json,sample_count,observed_slots,lineage_sha256 FROM aggregates WHERE resolution_seconds=60 AND period_start_ms>=? AND period_start_ms<? ORDER BY period_start_ms`, start, start+900000)
	if e != nil {
		return e
	}
	var count, slots int64
	sum := map[string]Statistic{}
	lineage := strings.Builder{}
	for rows.Next() {
		var raw, hash string
		var c, observed int64
		if e = rows.Scan(&raw, &c, &observed, &hash); e != nil {
			break
		}
		sub := map[string]Statistic{}
		if e = json.Unmarshal([]byte(raw), &sub); e != nil {
			break
		}
		for k, v := range sub {
			st := sum[k]
			merge(&st, v)
			sum[k] = st
		}
		count += c
		slots += observed
		lineage.WriteString(hash)
	}
	if err := rows.Err(); e == nil {
		e = err
	}
	rows.Close()
	if e != nil {
		return e
	}
	if count == 0 {
		return tx.Commit()
	}
	raw, e := json.Marshal(sum)
	if e != nil {
		return e
	}
	identifier := digest([]byte(fmt.Sprintf("%s:%d:900", s.id, start)))
	fin := Final{AgentID: s.id, SchemaVersion: 2, PeriodStartMS: start, ResolutionSeconds: 900, RecordID: identifier, SampleCount: count, ExpectedSlots: 900, ObservedSlots: slots, Summary: clone(sum)}
	unhashed, e := json.Marshal(fin)
	if e != nil {
		return e
	}
	hash := digest(unhashed)
	fin.PayloadSHA256 = hash
	wire, e := json.Marshal(fin)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO aggregates(resolution_seconds,period_start_ms,finalized_at_ms,sample_count,expected_slots,observed_slots,summary_json,quality_json,lineage_sha256,record_id,payload_sha256,wire_payload) VALUES(900,?,?,?,900,?,?,?,?,?,?,?)`, start, now, count, slots, string(raw), `{"policy":"1s/5s/30s"}`, digest([]byte(lineage.String())), identifier, hash, wire)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM aggregates WHERE resolution_seconds=60 AND period_start_ms>=? AND period_start_ms<?`, start, start+900000)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE server_info SET last_finalized_quarter_start_ms=? WHERE singleton_id=1 AND (last_finalized_quarter_start_ms IS NULL OR last_finalized_quarter_start_ms<?)`, start, start)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) OldestPending(ctx context.Context) (Pending, bool, error) {
	var p Pending
	e := s.db.QueryRowContext(ctx, `SELECT record_id,period_start_ms,payload_sha256,wire_payload FROM aggregates
 WHERE resolution_seconds=900 AND persisted_at_ms IS NULL ORDER BY period_start_ms LIMIT 1`).Scan(&p.RecordID, &p.PeriodStartMS, &p.Hash, &p.Payload)
	if errors.Is(e, sql.ErrNoRows) {
		return p, false, nil
	}
	return p, e == nil, e
}
func (s *Store) Acknowledge(ctx context.Context, id, hash string, at time.Time) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE aggregates SET persisted_at_ms=? WHERE resolution_seconds=900 AND record_id=? AND payload_sha256=? AND persisted_at_ms IS NULL`, at.UTC().UnixMilli(), id, hash)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("no matching unacknowledged final record")
	}
	_, e = tx.ExecContext(ctx, `UPDATE server_info SET last_ack_at_ms=CASE WHEN last_ack_at_ms IS NULL OR last_ack_at_ms<? THEN ? ELSE last_ack_at_ms END WHERE singleton_id=1`, at.UTC().UnixMilli(), at.UTC().UnixMilli())
	if e != nil {
		return e
	}
	return tx.Commit()
}

func fileBytes(path string) int64 {
	st, e := os.Stat(path)
	if e != nil {
		return 0
	}
	return st.Size()
}
func (s *Store) Status(ctx context.Context, now time.Time) (StorageStatus, error) {
	st := StorageStatus{AgentID: s.id, DBBytes: fileBytes(s.path), WALBytes: fileBytes(s.path + "-wal"), SHMBytes: fileBytes(s.path + "-shm"), RetentionCutoffMS: now.UTC().Add(-Retention).UnixMilli()}
	var stat syscall.Statfs_t
	if e := syscall.Statfs(filepath.Dir(s.path), &stat); e == nil {
		st.FilesystemFreeBytes = stat.Bavail * uint64(stat.Bsize)
		st.FilesystemTotalBytes = stat.Blocks * uint64(stat.Bsize)
	}
	items := []struct {
		query string
		dest  *int64
	}{
		{`SELECT count(*) FROM raw_samples WHERE saved=0`, &st.RawUnsaved},
		{`SELECT count(*) FROM raw_samples WHERE saved=1`, &st.RawSaved},
		{`SELECT count(*) FROM aggregates WHERE resolution_seconds=60`, &st.Minutes},
		{`SELECT count(*) FROM aggregates WHERE resolution_seconds=900`, &st.Finals},
		{`SELECT count(*) FROM aggregates WHERE resolution_seconds=900 AND persisted_at_ms IS NULL`, &st.Pending},
		{`SELECT count(*) FROM aggregates WHERE resolution_seconds=900 AND persisted_at_ms IS NOT NULL AND period_start_ms+900000<=?`, &st.EligibleFinals},
	}
	for i, it := range items {
		var e error
		if i == 5 {
			e = s.db.QueryRowContext(ctx, it.query, st.RetentionCutoffMS).Scan(it.dest)
		} else {
			e = s.db.QueryRowContext(ctx, it.query).Scan(it.dest)
		}
		if e != nil {
			return st, e
		}
	}
	var oldest sql.NullInt64
	if e := s.db.QueryRowContext(ctx, `SELECT min(period_start_ms) FROM aggregates WHERE resolution_seconds=900 AND persisted_at_ms IS NULL`).Scan(&oldest); e != nil {
		return st, e
	}
	if oldest.Valid {
		st.OldestPendingMS = &oldest.Int64
	}
	if e := s.db.QueryRowContext(ctx, `SELECT count(*) FROM raw_samples r WHERE r.saved=0 AND EXISTS (SELECT 1 FROM aggregates a WHERE a.resolution_seconds=60 AND a.period_start_ms=r.effective_at_ms/60000*60000)`).Scan(&st.LateRaw); e != nil {
		return st, e
	}
	var freePages, pageSize int64
	if e := s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freePages); e != nil {
		return st, e
	}
	if e := s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); e != nil {
		return st, e
	}
	st.SQLiteFreePageBytes = freePages * pageSize
	if e := s.db.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&st.SQLiteAutoVacuumMode); e != nil {
		return st, e
	}
	return st, nil
}
func (s *Store) Cleanup(ctx context.Context, now time.Time, dry bool, limit int) (CleanupResult, error) {
	if limit < 1 || limit > 5000 {
		return CleanupResult{}, errors.New("cleanup batch size out of range 1..5000")
	}
	pre, e := s.Status(ctx, now)
	if e != nil {
		return CleanupResult{}, e
	}
	res := CleanupResult{DryRun: dry, DBBytesBefore: pre.DBBytes}
	// In a dry run, counts refer to one bounded execution batch.
	count := func(query string, args ...any) (int64, error) {
		var v int64
		e := s.db.QueryRowContext(ctx, query, args...).Scan(&v)
		return v, e
	}
	res.Raw, e = count(`SELECT count(*) FROM (SELECT sample_id FROM raw_samples WHERE saved=1 LIMIT ?)`, limit)
	if e != nil {
		return res, e
	}
	res.Minutes, e = count(`SELECT count(*) FROM (SELECT 1 FROM aggregates m WHERE m.resolution_seconds=60 AND EXISTS
 (SELECT 1 FROM aggregates q WHERE q.resolution_seconds=900 AND q.period_start_ms=(m.period_start_ms/900000)*900000) LIMIT ?)`, limit)
	if e != nil {
		return res, e
	}
	res.Finals, e = count(`SELECT count(*) FROM (SELECT 1 FROM aggregates WHERE resolution_seconds=900 AND persisted_at_ms IS NOT NULL AND period_start_ms+900000<=? LIMIT ?)`, pre.RetentionCutoffMS, limit)
	if e != nil {
		return res, e
	}
	if dry {
		res.DBBytesAfter = res.DBBytesBefore
		return res, nil
	}
	// Each DELETE rechecks the invariants within SQLite, not a stale preview ID list.
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return res, e
	}
	defer tx.Rollback()
	queries := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM raw_samples WHERE sample_id IN (SELECT sample_id FROM raw_samples WHERE saved=1 LIMIT ?)`, []any{limit}},
		{`DELETE FROM aggregates WHERE (resolution_seconds,period_start_ms) IN
   (SELECT m.resolution_seconds,m.period_start_ms FROM aggregates m WHERE m.resolution_seconds=60 AND EXISTS
    (SELECT 1 FROM aggregates q WHERE q.resolution_seconds=900 AND q.period_start_ms=(m.period_start_ms/900000)*900000) LIMIT ?)`, []any{limit}},
		{`DELETE FROM aggregates WHERE (resolution_seconds,period_start_ms) IN
   (SELECT resolution_seconds,period_start_ms FROM aggregates WHERE resolution_seconds=900 AND persisted_at_ms IS NOT NULL
   AND period_start_ms+900000<=? LIMIT ?)`, []any{pre.RetentionCutoffMS, limit}},
	}
	dest := []*int64{&res.Raw, &res.Minutes, &res.Finals}
	for i, q := range queries {
		x, e := tx.ExecContext(ctx, q.sql, q.args...)
		if e != nil {
			return res, e
		}
		n, e := x.RowsAffected()
		if e != nil {
			return res, e
		}
		*dest[i] = n
	}
	if e = tx.Commit(); e != nil {
		return res, e
	}
	// Bounded best-effort page reclaim; never treat DELETE count as reclaimed disk bytes.
	_, _ = s.db.ExecContext(ctx, "PRAGMA incremental_vacuum(200)")
	_, _ = s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	res.DBBytesAfter = fileBytes(s.path)
	return res, nil
}
func (s *Store) Integrity(ctx context.Context) error {
	var status string
	if e := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&status); e != nil {
		return e
	}
	if status != "ok" {
		return fmt.Errorf("SQLite integrity_check: %s", status)
	}
	return nil
}
func (s *Store) Checkpoint(ctx context.Context) error {
	_, e := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	return e
}
func SortedKeys(m map[string]Statistic) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
