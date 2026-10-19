# Infrasible Agent — Operations

Staging-focused runbook. Local health does not prove central Relay/Persistence. See [STATUS.md](STATUS.md).

## Runtime

```sh
sudo systemctl status infrasible-agent --no-pager
sudo journalctl -u infrasible-agent -n 100 --no-pager
sudo systemctl restart infrasible-agent
```

Unit runs `infrasible-agent run --config=/etc/infrasible-agent/config.json` as user `infrasible-agent`, DB at `/var/lib/infrasible-agent/agent.db`. All channels outbound. Empty `monitoring_servers` = offline collection. Optional `enabled_families` selects metric modules at process start (omit = all default Linux families); config changes require `systemctl restart`. PKI rotation is not automated.

## Operator commands

```sh
sudo -u infrasible-agent infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage cleanup --safe --config=/etc/infrasible-agent/config.json
```

`health status` / `storage status` reflect local SQLite evidence. They do **not** prove Live socket state (`live_connection` may be `unknown_from_local_cli`). Use Relay metrics/logs for Live.

## Cleanup rules

- Unsaved raw and unACKed 900s finals stay protected.
- `--dry-run` / `--safe` are bounded batches; disk reclaim is best-effort.
- No general SQL or arbitrary shell from Desktop — authorized SSH only.

## Troubleshooting

- DB init failure: ownership + journalctl; do not delete the DB to “fix” identity.
- Live credentials rejected: client key must be regular file, mode `0600`, owned by service user.
- Pending forever: check Persistence ACK contract (HTTP 200 alone is insufficient).
- Live reconnect loops: Relay must accept Hello then `type: live` and send heartbeats — [MONITORING_RELAY.md](design/MONITORING_RELAY.md).
- Config changed but metrics/endpoints unchanged: no hot reload — validate, then `systemctl restart infrasible-agent`. Confirm `enabled_families` in startup journal and in `config validate` JSON.
