// This file is tested and fixed. Do not add, remove, or change the code in this file.
package storage

import (
	"context"
	"database/sql"
	"time"
)

// Health exposes observable local data freshness without falsely claiming that
// a separate CLI invocation proves the daemon process or central channels live.
type Health struct {
	AgentID            string `json:"agent_id"`
	CheckedAtMS        int64  `json:"checked_at_ms"`
	LastRawMS          *int64 `json:"last_raw_ms,omitempty"`
	LastMinuteStartMS  *int64 `json:"last_minute_start_ms,omitempty"`
	LastQuarterStartMS *int64 `json:"last_quarter_start_ms,omitempty"`
	LastDurableACKMS   *int64 `json:"last_durable_ack_ms,omitempty"`
	CollectorFresh     bool   `json:"collector_fresh"`
	PendingCount       int64  `json:"pending_count"`
	OldestPendingMS    *int64 `json:"oldest_pending_ms,omitempty"`
	SQLiteReady        bool   `json:"sqlite_ready"`
	// Live/Persistence connectivity is runtime-only, not proven by an offline CLI.
	LiveConnection        string `json:"live_connection"`
	PersistenceConnection string `json:"persistence_connection"`
}

// Health builds a local freshness report from SQLite without asserting Live or
// Persistence channel connectivity from a separate CLI process.
func (s *Store) Health(ctx context.Context, now time.Time) (Health, error) {
	st, e := s.Status(ctx, now)
	if e != nil {
		return Health{}, e
	}
	h := Health{AgentID: s.id, CheckedAtMS: now.UTC().UnixMilli(), PendingCount: st.Pending,
		OldestPendingMS: st.OldestPendingMS, SQLiteReady: true,
		LiveConnection: "unknown_from_local_cli", PersistenceConnection: "unknown_from_local_cli"}
	fetch := func(query string, out **int64) error {
		var v sql.NullInt64
		if e := s.db.QueryRowContext(ctx, query).Scan(&v); e != nil {
			return e
		}
		if v.Valid {
			*out = &v.Int64
		}
		return nil
	}
	for _, v := range []struct {
		query string
		out   **int64
	}{
		{"SELECT max(observed_at_ms) FROM raw_samples", &h.LastRawMS},
		{"SELECT last_finalized_minute_start_ms FROM server_info WHERE singleton_id=1", &h.LastMinuteStartMS},
		{"SELECT last_finalized_quarter_start_ms FROM server_info WHERE singleton_id=1", &h.LastQuarterStartMS},
		{"SELECT max(persisted_at_ms) FROM aggregates WHERE resolution_seconds=900", &h.LastDurableACKMS},
	} {
		if e = fetch(v.query, v.out); e != nil {
			return Health{}, e
		}
	}
	if h.LastRawMS != nil {
		age := h.CheckedAtMS - *h.LastRawMS
		h.CollectorFresh = age >= 0 && age <= 10000
	}
	return h, nil
}
