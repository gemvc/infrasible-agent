# Live Hello and Heartbeat — Implementation Guide

**Status:** planned next Agent slice after staging-ready v0.3.1 and mock Bridge/Persistence contract tests  
**Scope:** outbound Live WSS only (`internal/transport`)  
**Canonical target envelopes:** [AGENT_PROTOCOL_AND_SECURITY.md](AGENT_PROTOCOL_AND_SECURITY.md) §4–5  
**Executable baseline today:** [AGENT_PROTOCOL_AND_SECURITY.md](AGENT_PROTOCOL_AND_SECURITY.md) §12 (`type: live` frames, no Hello)

This document tells implementers **how** to add Live Hello negotiation and application-level heartbeat without rewriting Persistence, SQLite, or collector code. It also records the **staging host preparation** operators should complete before exercising Live against a real or mock Bridge.

## 1. Staging preparation and install

**Yes — install on a staging/test Linux host** to prove offline collection and later Hello against a mock or lab Bridge. **Do not** install on a production monitoring path yet: Hello is not implemented in the current binary, and central Bridge/PKI interoperability is not certified.

Authoritative install detail: [INSTALLATION.md](../INSTALLATION.md). Summary below.

### 1.1 When this phase applies

| Mode | Recommended? | Config |
|---|---|---|
| Offline staging (local SQLite only) | **Yes** — prep phase now | `"monitoring_servers": []` |
| Live Hello against lab/mock Bridge | After this Hello slice lands | One `live_url` + mTLS files |
| Production Monitoring Relay | **No** until Hello + external sign-off | — |

### 1.2 Preparation checklist

1. **Host:** Ubuntu/Debian Linux (amd64 or arm64), disposable or non-critical.
2. **Artifact:** Latest successful [Agent CI](https://github.com/gemvc/infrasible-agent/actions/workflows/ci.yml) → Artifacts → `linux-packages`, or build with `bash scripts/package-linux.sh` and copy the matching `.deb`.
3. **Architecture:** `uname -m` and `dpkg --print-architecture` → use `infrasible-agent_0.3.1_amd64.deb` or `_arm64.deb`.
4. **Checksum:** verify the chosen file against `SHA256SUMS` (integrity only; not publisher authentication).
5. **Expectations:** default config has empty `monitoring_servers` — no Relay, no certificates required for offline staging.

### 1.3 Install (Debian/Ubuntu)

```sh
sudo apt install ./infrasible-agent_0.3.1_amd64.deb   # or arm64 package
sudo /usr/bin/infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl enable --now infrasible-agent
sudo systemctl status infrasible-agent --no-pager
```

The package `postinst` creates the `infrasible-agent` system user, `/var/lib/infrasible-agent` (0750), `/etc/infrasible-agent`, and installs the example config only if `/etc/infrasible-agent/config.json` is absent. It does **not** enable or start the service automatically.

### 1.4 Validate offline collection

Wait for the daemon to create the database, then:

```sh
sudo journalctl -u infrasible-agent -n 50 --no-pager
sudo -u infrasible-agent /usr/bin/infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
```

Empty minute/quarter aggregates are normal until the UTC grace window (~2 minutes after period end). Do not treat early empty rollups as a failed install.

### 1.5 Before pointing at a Live Bridge

Keep `monitoring_servers` empty until:

1. This Hello/heartbeat slice is **implemented** and tested (sections below).
2. Lab Bridge (or updated mock) accepts `agent_hello` then `type: live`.
3. mTLS materials and `server_name` are staged per [CONFIGURATION.md](../CONFIGURATION.md).

Adding Production Relay URLs without Hello and joint contract alignment will not deliver a supported Live path.

## 2. Goal

After a successful mTLS WebSocket dial to `live_url`:

1. Send `agent_hello`.
2. Wait for `hello_accepted` (fail closed on timeout, mismatch, or unauthorized identity/channel).
3. Stream existing v0.3.x Live frames (`type: "live"`) unchanged in this slice.
4. Send application heartbeats on the interval from `hello_accepted.heartbeat_seconds` (default **15**).
5. Reconnect if the peer goes silent for approximately **3×** the heartbeat interval.

Persistence HTTPS/ACK behavior is **out of scope** for this document.

## 3. Non-goals (same PR)

| Out of scope | Reason |
|---|---|
| Rename Live frames to `live_sample` | Separate wire migration; keep `type: live` for temporary Bridge compatibility |
| Persistence multi-URL failover | Depends on verified domain/Hello authority |
| PKI enrollment / cert rotation | External issuer contract |
| Config hot-reload | Separate lifecycle ticket |
| Changes to frozen `storage`, `collector`, or daemon scheduler | Not required for Hello |

## 4. Files to touch

| Path | Change |
|---|---|
| [`internal/transport/transport.go`](../../internal/transport/transport.go) | **Unfreeze** for this change set. Add negotiate + heartbeat; keep frame JSON shape. |
| [`internal/transport/contract_test.go`](../../internal/transport/contract_test.go) | Mock Bridge: Hello first, then `live`, plus negative and heartbeat cases. |
| Optional: `Sender` construction in [`internal/agent/agent.go`](../../internal/agent/agent.go) | Pass `AgentVersion` / protocol constants into `Sender` if Hello needs them (unfreeze agent only if injection is cleaner than local constants). |

Do **not** edit sealed production files that are unrelated. Re-freeze `transport.go` (and agent if touched) after tests pass.

## 5. Target wire (Hello)

### Agent → Bridge

```json
{
  "type": "agent_hello",
  "protocol_major": 2,
  "protocol_minor": 0,
  "agent_id": "<stable-agent-id>",
  "agent_version": "<semver>",
  "channel": "live",
  "schema_versions": ["2.0"],
  "capabilities": ["live_1hz", "sample_quality", "metric_cadence_v1", "metric_freshness_v1"],
  "sent_at_utc": "<RFC3339 UTC>"
}
```

### Bridge → Agent

```json
{
  "type": "hello_accepted",
  "protocol_major": 2,
  "protocol_minor": 0,
  "schema_version": "2.0",
  "authorized_agent_id": "<stable-agent-id>",
  "authorized_channel": "live",
  "endpoint_id": "monitor-a",
  "heartbeat_seconds": 15,
  "max_message_bytes": 262144
}
```

### Acceptance rules (Agent fail-closed)

Reject and reconnect when any of the following hold:

- No readable message within a bounded timeout (recommended **10s**).
- `type` ≠ `hello_accepted`.
- `protocol_major` ≠ `2`.
- `authorized_agent_id` ≠ local `Store.AgentID()`.
- `authorized_channel` ≠ `"live"`.
- `heartbeat_seconds` ≤ 0 or unreasonably large (clamp or reject; document chosen bound in tests).
- Payload larger than `max_message_bytes` when that field is present (optional in first slice if size enforcement is deferred — note the deferral in the PR).

A `hello_accepted` envelope alone does **not** replace mTLS or registrar ACL; it only confirms the negotiated Live session parameters.

## 6. Connect sequence (after this slice)

```text
select LiveURL candidate
  → mTLS Dial WSS
  → Write agent_hello
  → Read hello_accepted (fail closed)
  → loop:
        Write type=live frames from bounded channel
        Write heartbeat every heartbeat_seconds
        if peer silent ≈ 3× heartbeat_seconds → close, backoff, reconnect
```

Today’s reconnect delay (~3s) may remain as the initial backoff between dial attempts.

## 7. Implementation sketch

Suggested helpers on `Sender` (names indicative):

1. **`negotiateLive(ctx, conn, server) (heartbeat time.Duration, err)`**  
   Build Hello from `Store.AgentID()`, agent version, protocol 2.0; Write; Read one text message; validate; return heartbeat duration (default 15s if field omitted — prefer requiring the field).

2. **`stream(ctx, conn, frames, heartbeat)`**  
   `select` on `ctx.Done()`, frame channel, and heartbeat ticker. Heartbeat JSON candidate:

   ```json
   { "type": "heartbeat", "sent_at_utc": "<RFC3339 UTC>" }
   ```

   Document the chosen heartbeat type in contract tests. Do not invent metric refreshes from heartbeats.

3. **`Live`**  
   After Dial success, call `negotiateLive` before `stream`. On negotiate or stream error, close socket, log, advance candidate index / backoff as today.

### Frame compatibility

Continue marshaling Live samples as:

```json
{ "type": "live", "agent_id": "...", "frame": { ... } }
```

Switching to `live_sample` (§5 target) is a **follow-up** document/PR after Hello is stable with Bridge owners.

### Version injection

Prefer:

```go
type Sender struct {
    Servers      []config.Server
    Store        *storage.Store
    Log          *slog.Logger
    AgentVersion string // e.g. "0.3.1"
}
```

Set from `main`/`agent` using existing `agentVersion`. Avoid forking protocol semantics in two packages.

## 8. Mock Bridge contract tests (write/update first)

Extend [`internal/transport/contract_test.go`](../../internal/transport/contract_test.go):

| Case | Expected |
|---|---|
| Happy path | First message is `agent_hello`; mock replies `hello_accepted`; then one `type: live` frame accepted |
| No Hello reply | Agent fails negotiate; reconnects; no false durability (Live never sets `persisted_at`) |
| Wrong `authorized_agent_id` | Negotiate fails; reconnect |
| Wrong channel | Negotiate fails |
| Heartbeat | After accept, mock observes a heartbeat within `heartbeat_seconds + ε` |
| No history | Mock remains transient RAM only (unchanged invariant) |

Update existing `TestMockBridgeAcceptsLiveEnvelopeAndStoresNoHistory` so it no longer assumes the **first** message is `type: live`.

## 9. Verification

```sh
go test -race -count=1 ./internal/transport/...
go test -race -count=1 ./...
go vet ./...
```

CI packaging need not change for this protocol-only slice.

## 10. Documentation updates when implemented

When the code lands:

1. Mark this guide **implemented** (date + commit SHA).
2. Update [AGENT_PROTOCOL_AND_SECURITY.md](AGENT_PROTOCOL_AND_SECURITY.md) §12 Live subsection: Hello/heartbeat are present; frames remain `type: live` until a later migration.
3. Note temporary Bridge compatibility: Relays must accept Hello then `live`, or provide an explicit adapter.
4. Re-freeze touched production `.go` files after tests pass.

## 11. Ordering relative to later work

```text
[done] staging-ready agent + freeze
[done] mock Bridge/Persistence contract tests (§12)
[now]  staging host offline install (section 1)
[this] Live Hello + heartbeat
  → optional: migrate frames to live_sample
  → Persistence failover (domain-safe)
  → PKI enrollment / rotation (external)
```

## 12. References

- Install / staging: [INSTALLATION.md](../INSTALLATION.md), [CONFIGURATION.md](../CONFIGURATION.md), [DAEMON_RUNBOOK.md](../DAEMON_RUNBOOK.md)  
- Hello / Live target: [AGENT_PROTOCOL_AND_SECURITY.md](AGENT_PROTOCOL_AND_SECURITY.md) §4–5  
- Current executable wire: same doc §12  
- Backlog tickets: AG-0604, AG-0703 (Hello/authz and reconnect/heartbeat)  
- Mock fixtures already in tree: `internal/transport/contract_test.go`
