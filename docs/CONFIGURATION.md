# Agent configuration

Daemon reads `/etc/infrasible-agent/config.json` (`config_version: 1`). Unknown fields fail validation. Config is loaded at process start (no hot reload); apply changes with validate + systemd restart.

## Offline staging (default)

```json
{
  "config_version": 1,
  "data_path": "/var/lib/infrasible-agent/agent.db",
  "monitoring_servers": []
}
```

Omitting `enabled_families` enables the full default Linux host set: `cpu`, `memory`, `network`, `disk_io`, `load`, `filesystem`.

## Metric families

```json
{
  "config_version": 1,
  "data_path": "/var/lib/infrasible-agent/agent.db",
  "enabled_families": ["cpu", "memory", "network", "disk_io", "load", "filesystem"],
  "monitoring_servers": []
}
```

| Field | Notes |
|---|---|
| `enabled_families` | Optional. Omitted/`null` → all default Linux families. Must be non-empty when set; names must be unique and in the collector catalog. Unknown names fail validation. |
| Cadence (built-in) | `cpu`/`memory`/`network`/`disk_io` = 1s; `load` = 5s; `filesystem` = 30s |

Future families (for example `gpu`, `docker`) are added as compile-time Go modules and catalog names; operators enable them here after they ship. See [design/COLLECTOR_FAMILIES.md](design/COLLECTOR_FAMILIES.md).

**Restart required:** changing `enabled_families` (or any other config field) does not apply until the daemon restarts. `config validate` always reports `restart_required: true`.

## Live / Persistence server entry

```json
{
  "id": "monitor-primary",
  "live_url": "wss://relay.example.com/v2/agents/live",
  "persistence_url": "https://persist.example.com/v2/agents/aggregates",
  "persistence_domain": "primary",
  "server_name": "relay.example.com",
  "ca_file": "/etc/infrasible-agent/ca.pem",
  "client_cert_file": "/var/lib/infrasible-agent/client.crt",
  "client_key_file": "/var/lib/infrasible-agent/client.key"
}
```

| Field | Notes |
|---|---|
| `monitoring_servers` | Ordered; unique `id`; empty = offline only |
| `live_url` | Optional; if set, must be `wss://` |
| `persistence_url` | Optional; if set, must be `https://`; requires `persistence_domain` |
| `server_name` | TLS hostname/SNI |
| `ca_file` / cert / key | Paths only; **key mode `0600`, owner = service user** |

Live and Persistence are separate. Persistence currently uses the first configured URL only (no domain-safe failover yet).

```sh
sudo -u infrasible-agent infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl restart infrasible-agent
```

Successful validate JSON includes `enabled_families`, `restart_required: true`, and a message reminding operators to restart.

## Desktop → Agent → Relay (operator flow)

| Actor | Role |
|---|---|
| Desktop | SSH/SFTP install & config; never opens Live as management |
| Agent | Collects; dials out Live + Persistence |
| Monitoring Relay | AuthZ + fan-out live frames — see [MONITORING_RELAY.md](design/MONITORING_RELAY.md) |
| Persistence API | Durable 900s finals + ACK |

1. Install package ([INSTALLATION.md](INSTALLATION.md)); start once with empty servers → stable `agent_id`.  
2. Enroll with central registrar/PKI (**external**; Agent does not self-enroll).  
3. Stage CA + client cert + key (key `0600`).  
4. Write config with registered `live_url` / optional Persistence; validate; restart.  
5. Verify local health/storage; confirm Live on **Relay** side (CLI cannot prove socket).  
6. Desktop views live via Relay subscribers; history via Historical API — not Agent SQLite over the network.

### Blocking external decisions

Registrar/PKI, Relay subscriber auth, Persistence ACK fixtures, persistence-domain HA. None are “done” because Agent CI is green. Wire detail: [AGENT_PROTOCOL_AND_SECURITY.md](design/AGENT_PROTOCOL_AND_SECURITY.md) §12. Status: [STATUS.md](STATUS.md).
