# Agent configuration — v0.2.0

The daemon reads `/etc/infrasible-agent/config.json`. The format has `config_version: 1` and is JSON. Unknown fields fail validation. The current implementation reads the config at process start: **automatic atomic reload and desired-vs-effective status are not implemented**. A validated change requires a controlled systemd restart.

```json
{
  "config_version": 1,
  "data_path": "/var/lib/infrasible-agent/agent.db",
  "monitoring_servers": []
}
```

`data_path` is the SQLite database filename; default is `/var/lib/infrasible-agent/agent.db` if omitted. Keep it on a durable local filesystem writable by the daemon user. Store the config as root-owned, daemon-group-readable mode 0640; the SQLite directory should be accessible only to the service account and authorized operators.

A configured server entry has the following shape; endpoint values are **illustrative, not provisioned endpoints**:

```json
{
  "id": "monitor-primary",
  "live_url": "wss://monitor.example.invalid/v2/agents/live",
  "persistence_url": "https://monitor.example.invalid/v2/agents/aggregates",
  "persistence_domain": "primary",
  "server_name": "monitor.example.invalid",
  "ca_file": "/etc/infrasible-agent/ca.pem",
  "client_cert_file": "/var/lib/infrasible-agent/client.crt",
  "client_key_file": "/var/lib/infrasible-agent/client.key"
}
```

The actual JSON places server objects in the `monitoring_servers` array. `live_url` must use `wss://` and `persistence_url` must use `https://`. When either channel is configured the server also needs a name and CA/cert/key file paths; Persistence requires `persistence_domain`. Entries must have unique IDs. The current code validates the shape but **does not fully verify file ownership, issuer authorization, centrally registered storage-domain trust, or automatic certificate renewal**.

The current Persistence sender selects a single configured Persistence endpoint; do not assume redundant endpoints provide authorized durable failover. No credentials are embedded in config; use protected file paths. Endpoint/channel authorization remains subject to signed central contracts.

Validate prior to restart:

```sh
sudo -u infrasible-agent infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl restart infrasible-agent
```

Runtime memory/CPU quotas must be applied and verified by the privileged operating-system operator via systemd/cgroups. Host CPU/RAM/disk *alert thresholds* are a separate Desktop policy, not daemon resource limits. An Agent storage budget must **never** bypass protected Pending records.
