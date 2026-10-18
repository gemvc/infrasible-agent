# Agent configuration — v0.3.0

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


## Desktop → Agent → Monitoring Relay: operator integration (v0.3.0)

This is an **integration handoff**, not a claim that the Transible Desktop UX or central Relay already implements these steps. [Protocol/security](design/AGENT_PROTOCOL_AND_SECURITY.md) is the canonical target contract; this document distinguishes executable v0.3.0 CLI behavior from requirements for later implementation.

### Roles and routes

| Actor | Responsibility | Connection |
|---|---|---|
| Transible / Infrasible Monitoring Desktop | Operator selects monitored Linux host and registered Monitoring endpoint, obtains authorization, installs/configures Agent, inspects local health/storage and performs explicitly confirmed safe maintenance | Existing Desktop-managed SSH/SFTP/Ansible only for host management. **Do not change core SSH** |
| Linux Agent | Collects and stores samples; opens outgoing connections; owns local stable `agent_id` | WSS/mTLS → Live Relay; HTTPS/mTLS → separate Persistence API |
| Live Relay | Authenticates/authorizes Agent, accepts fresh live frames and forwards them to subscribed desktop live viewers; ephemeral connections/subscriptions only | Agent → Relay → authenticated Desktop subscription |
| Persistence API | Accepts only final 15-minute aggregates, performs idempotent durable PostgreSQL commit, returns verifiable durable ACK | Agent → Persistence API; Historical Desktop requests → separate Historical API |

A **single currently active relay** is the intended normal Live path, not multiple simultaneous relay fanouts. The approved configuration permits an ordered list of registered endpoints; priority/failover support is an **incomplete target**, not a promise of v0.3.0. The Desktop must not open/close SSH once per second or request historical data from the Live Relay. The Agent has no inbound management port. The central Historical API and any subscriber handshake are **external contracts not implemented in this repository**.

### Operator provisioning sequence — supported vs planned

1. **Desktop host selection [Desktop integration required]:** select one existing SSH-connected Linux host, display its identity and require an authorized operator action; do not copy Desktop vault secrets into Agent JSON.
2. **Installation [documented local procedure]:** transfer a verified architecture-matched Agent package over existing SSH/SFTP, create dedicated service ownership and install the provided systemd service as described in [Installation](INSTALLATION.md). Package checksum alone does not attest the publisher; release-signature/provenance validation remains a release gate.
3. **First local run [implemented]:** configure `config_version=1`, `data_path` and initially `monitoring_servers: []` for **offline test only**; start service once to bootstrap its stable `agent_id` in SQLite. Never delete DB or register a second identity to "fix" network issues.
4. **Central host enrollment [NOT IMPLEMENTED]:** Desktop/operator must establish authorization with the Monitoring control plane to associate that Agent ID and host with an approved endpoint, channel permissions and issuer. The central registration API, CSR submission, one-time token, certificate issuance/rotation/revocation and Desktop UI are **not specified as working endpoints**. Do **not** invent credentials, self-sign an identity and call it centrally authorized, or assume simply entering a server URL constitutes registration.
5. **Distribute trusted material [manual staging only]:** provision the *central-authorized* trust anchor, issued client cert and matching private key as OS-protected files. Private key generation on the monitored host and no plaintext key transfer is the target. The issuer's binding of cert ↔ `agent_id` ↔ host ↔ channel must be externally verified.
6. **Configure endpoints [implemented parser]:** construct the versioned JSON shown below from registered server data, not arbitrary UI text; priority is the array order. Current code validates URI scheme and required field presence, **not** CA contents, certificate authorization, ownership or persistence-domain federation.
7. **Activate [implemented with service restart]:** stage a root-authorized JSON update through existing SSH, validate it using `infrasible-agent config validate --config=...`, then replace the file through authorized OS operations and restart systemd. **Automatic daemon hot reload, effective-config revision status and rollback orchestration are not implemented.** Preserve the previous config to restore on a failed restart.
8. **Verify [partially implemented]:** inspect service logs, `health status`, and `storage status` via existing SSH. These local commands do **not** attest that the Relay accepted Agent registration or that the Persistence API committed a record. A real end-to-end check requires Relay subscription/authorization evidence and a durable ACK emitted by the matching central API.
9. **View data [external integration required]:** Desktop connects to the authorized Live Relay subscriber interface for current fresh frames; it calls the **Historical API** for persisted history. It may invoke local safe cleanup only via the existing SSH operator path after explicit confirmation, never over WSS/HTTPS ingestion.

### Exact v0.3.0 Agent JSON example

The domains ending in `.invalid` are **non-functional placeholders**. Replace them only with centrally registered URLs and certificate paths. The paths `/v2/agents/live` and `/v2/agents/aggregates` are example paths from the architecture, **not proven server routes**.

```json
{
  "config_version": 1,
  "data_path": "/var/lib/infrasible-agent/agent.db",
  "monitoring_servers": [
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
  ]
}
```

**Configuration field contract (source: `internal/config/config.go`):**

| Field | Meaning and current validation |
|---|---|
| `config_version` | Must equal integer `1`; unknown JSON fields are rejected |
| `data_path` | Durable local SQLite location; default `/var/lib/infrasible-agent/agent.db` when omitted |
| `monitoring_servers` | Ordered array; unique non-empty `id`; either or both channels allowed. Empty means offline test only |
| `live_url` | Optional, but when set must be a valid `wss://` URL; used for Agent-originated live ingress |
| `persistence_url` | Optional, but when set must be a valid `https://` URL; **separate function** from Relay history |
| `persistence_domain` | Required with Persistence URL; a string alone **does not establish** shared durable/dedup authority |
| `server_name` | TLS certificate hostname/SNI expectation |
| `ca_file` | PEM trust-root path |
| `client_cert_file`, `client_key_file` | mTLS client identity paths; avoid embedding secret bytes in JSON |

**Current endpoint behavior:** Live tries configured Live URLs on connection failures, cycling with a fixed 3-second retry; it does **not** implement the approved authenticated Hello/ACL negotiation, priority-preserving stable failback or a trusted registration proof. Persistence chooses the **first** configured Persistence URL and retries pending data there at approximately 5-second intervals; it does **not** fail over to later Persistence endpoints. Do not present multiple configured entries as working HA.

### Desktop local actions and authorization boundary

```sh
# On the selected Linux host, invoked by an already-authorized operator over existing SSH:
sudo -u infrasible-agent infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl restart infrasible-agent
sudo systemctl status infrasible-agent --no-pager
sudo -u infrasible-agent infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
# Require an operator confirmation after preview:
sudo -u infrasible-agent infrasible-agent storage cleanup --safe --config=/etc/infrasible-agent/config.json
```

These are fixed local executable names and arguments—not a request to allow arbitrary shell strings. Desktop must use its existing command authorization/privilege model, distinguish a desired config from the daemon's actual running config, redact private data from logs and record who approved changes. The CLI's local status reports SQLite-related evidence, **not live socket connection state**. A cleanup is bounded and must protect unsaved samples and unACKed final aggregates; a dry-run does not guarantee reclaimed file bytes.

### Required Desktop UX states (target, not implemented here)

`NOT_INSTALLED` → `INSTALLED_OFFLINE` → `ENROLLMENT_REQUIRED` → `CREDENTIALS_STAGED` → `CONFIG_VALIDATED` → `SERVICE_RUNNING` → `LIVE_AUTHORIZED` / `PERSISTENCE_AUTHORIZED`. Show Live and Persistence independently; a running systemd unit is not evidence of either authorization. Also distinguish `INVALID_CONFIG`, `CERT_EXPIRED`, `AUTH_REJECTED`, `RELAY_DISCONNECTED`, `PERSISTENCE_PENDING`, `STORAGE_DEGRADED` and `STALE_DATA`. These states require implementation and reliable evidence sources on the Desktop/central side; they are not current Agent status-CLI output.

### Blocking handoff decisions / cross-repository contract acceptance

Before enabling real server registration or advertising end-to-end monitoring, interface owners must provide: the canonical live ingress/subscriber schemas and authorization mechanism; the central registrar/PKI issuance, renewal and revocation interfaces; trusted `agent_id` enrollment binding; exact HTTPS final payload plus durable ACK, checksum and idempotency fixtures; verified persistence-domain HA rules; the Desktop's minimal setup/status workflow and trusted approval/safe-cleanup semantics. Keep the proposed version-2 envelopes in [Protocol/security](design/AGENT_PROTOCOL_AND_SECURITY.md) distinct from the actual v0.3.0 wire described there. None of these absent services should be claimed operational solely because local Agent CI passes.
