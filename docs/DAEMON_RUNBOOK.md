# Infrasible Agent v2 — daemon operator runbook

This is an early functional implementation, **not a production acceptance certificate**. The central Bridge, Persistence, registration authority, authenticated deduplication-domain attestations, and Desktop UI are external and are not shipped in this repository. G00 and external API/security signoffs remain necessary before deployment.

## Prepare a disposable Linux machine

Use a dedicated non-root `infrasible-agent` user. Install the native Linux binary via the tarball or generated .deb. Copy `packaging/infrasible-agent.service` to the systemd unit path if not already installed. Create `/etc/infrasible-agent/config.json` based on `packaging/config.example.json`, owned by root and readable by the daemon service account. Configure real registered mTLS endpoints and certificates only after external protocol confirmation. The daemon works locally with an empty `monitoring_servers` array for offline storage/aggregation validation.

```sh
sudo useradd --system --home-dir /var/lib/infrasible-agent --shell /usr/sbin/nologin infrasible-agent
sudo install -d -m 0750 -o infrasible-agent -g infrasible-agent /var/lib/infrasible-agent
sudo install -d -m 0750 /etc/infrasible-agent
sudo install -m 0640 -o root -g infrasible-agent packaging/config.example.json /etc/infrasible-agent/config.json
sudo systemctl daemon-reload
sudo systemctl enable --now infrasible-agent
sudo journalctl -u infrasible-agent -f
```

Do not run this installer on production until its service user, certificate paths and interface contract are verified. The example is intentionally disconnected and cannot deliver history or Live.

## Local maintenance over existing authorized SSH

```sh
infrasible-agent config validate --config=/etc/infrasible-agent/config.json
infrasible-agent storage status --config=/etc/infrasible-agent/config.json
infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
infrasible-agent storage cleanup --safe --config=/etc/infrasible-agent/config.json
```

Access to the SQLite directory determines maintenance permissions. Never grant generic SQL execution to Desktop users. A dry run produces counts for a limited batch and estimates no guaranteed byte reclamation. Each execute batch rechecks protected-row conditions. Repeat explicitly if necessary.

`raw_samples.saved=0` and unacknowledged 15-minute finals are always protected. ACKed finals older than 60 days from UTC period **end** are eligible. The database can hold pending history longer than 60 days and may fill; storage pressure is reported as a degradation, never silently resolved by deleting Pending.

When troubleshooting, keep the main database and WAL together; do not copy just the main DB while SQLite is active. Never erase the DB on a failed migration/integrity check.

## Operational limitations

- This initial release candidate does not enroll or rotate certificates automatically. Credentials must be provisioned by an authorized operator.
- Persistence uses a single pinned server until a verifiable shared deduplication-domain attestation mechanism is approved; a matching string in config does not prove cross-server durability.
- The proposed ACK JSON is not a released central API contract. Never interpret HTTP 200 alone as durable success.
- Production resource/soak, power-loss and native ARM acceptance tests are not signed off.
- User-facing Storage Dashboard, thresholds and confirmation UX are work in the separate Desktop repository.
