# Infrasible Agent v0.3.0 — Operations

> Test and staging only. This is a local daemon/runbook, **not a certificate of central Bridge/Persistence interoperability**.

## Runtime

```sh
sudo systemctl status infrasible-agent --no-pager
sudo journalctl -u infrasible-agent -n 100 --no-pager
sudo systemctl restart infrasible-agent
```

The systemd unit runs `/usr/bin/infrasible-agent run --config=/etc/infrasible-agent/config.json` under dedicated `infrasible-agent` credentials and stores data in `/var/lib/infrasible-agent/agent.db`. All network channels are outbound. The daemon continues collecting locally when `monitoring_servers` is empty. Network registration and PKI rotation are **not automated** in v0.3.0.

## Operator commands (authorized local or existing SSH session)

```sh
sudo -u infrasible-agent infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage cleanup --safe --config=/etc/infrasible-agent/config.json
```

Status reports database, WAL and SHM bytes, filesystem capacity/free, raw saved and unsaved counts, minute and final counts, oldest Pending, eligible expired ACKed finals, SQLite free pages and late raw samples. `health status` reports SQLite readiness and last known local raw/aggregate/ACK times; **it explicitly cannot prove Live or Persistence connectivity from a separate CLI invocation**. Use centralized monitoring and logs for those channels.

## Storage lifecycle and cleanup

- Unsaved raw frames remain protected, even when late or excluded from an already immutable aggregate.
- Raw rows become `saved=1` only when an eligible minute summary is committed, and can then be cleaned up in bounded batches.
- 60-second rows are eligible for cleanup only when their enclosing 900-second immutable final has been committed.
- 900-second final rows are eligible only after a matching durable ACK **and** 60 days counted from UTC period end. Pending final records are protected indefinitely from ordinary cleanup, even under low disk conditions.
- `--dry-run` estimates a **bounded batch (up to 500 rows per category)**, not total reclaimable disk bytes. `--safe` rechecks predicates in SQLite; deletions do not necessarily shrink DB/WAL immediately. SQLite page vacuum/checkpoint are best-effort, not synchronous guaranteed reclamation.

**Do not grant Desktop a general SQL execution interface, or let arbitrary shell input reach maintenance.** Require human authorization for destructive maintenance, and preserve audit information at the Desktop/operator layer. Running the command over the *existing* SSH connection does not require a separate management port or per-second SSH polling.

## Troubleshooting

- If database initialization fails, inspect service-user ownership and `journalctl` before retrying. Do **not** remove the SQLite database to mask a schema/integrity error.
- If history remains Pending, inspect mTLS credentials and registered central API/ACK semantics; an HTTP 200 alone is not durable proof.
- Always back up SQLite consistently, accounting for WAL and SHM; copying only `agent.db` while the daemon is active can lose committed data.
- Do not publish an unattended auto-updater or treat the unsigned checksum file as publisher authentication.

See [Installation](INSTALLATION.md), [Configuration](CONFIGURATION.md) and the [acceptance matrix](design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md).
