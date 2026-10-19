# Linux installation — Infrasible Agent v0.3.1

> **Pre-production testing only.** v0.3.1 is an implementation candidate. It can collect and aggregate locally with no monitoring endpoints; it is not a fully integrated central monitoring deployment.

## Download a built artifact

Use the latest **successful main** [Agent CI run](https://github.com/gemvc/infrasible-agent/actions/workflows/ci.yml), open its **Artifacts** section and download `linux-packages` ZIP. A successful CI run produces the native Linux executables, tarballs, Debian packages on amd64/arm64/armhf, an x86_64 RPM, `SHA256SUMS` and per-binary `.sha256` files. **CI artifacts are not automatically a GitHub Release.**

When a manually approved `v0.3.1` release has been published, its verified assets can instead be obtained from [Releases](https://github.com/gemvc/infrasible-agent/releases). Never install the old `v0.1.0` asset expecting daemon-v2 functionality.

Match the Linux OS architecture using `uname -m` and `dpkg --print-architecture`. Use `infrasible-agent_0.3.1_amd64.deb` on Debian/Ubuntu amd64 or `infrasible-agent_0.3.1_arm64.deb` on arm64. On other Linux targets use the matching tarball; generated RPMs only package the binary, not systemd/configuration.

Verify checksums for the downloaded matching package from `SHA256SUMS` when present. SHA-256 alone does **not** authenticate the publisher or protect against a compromised CI account.

## Install on Ubuntu/Debian staging host

After downloading/extracting artifacts to the Linux machine:

```sh
sudo apt install ./infrasible-agent_0.3.1_amd64.deb
sudo /usr/bin/infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl enable --now infrasible-agent
sudo systemctl status infrasible-agent --no-pager
```

The Debian package `postinst` creates the `infrasible-agent` system user, `/var/lib/infrasible-agent` (0750), `/etc/infrasible-agent`, and installs `config.example.json` as `/etc/infrasible-agent/config.json` only when that file is absent. It does **not** enable or start the service; that remains an operator choice. The sample config uses an empty `monitoring_servers` list for **offline staging collection only**. It does not enroll certificates or connect to a production Monitoring relay.

For tarballs, install the binary to `/usr/bin/infrasible-agent`, its `infrasible-agent.service` to `/etc/systemd/system/infrasible-agent.service`, create the same user and directories manually, and copy `config.example.json` to `/etc/infrasible-agent/config.json` with ownership `root:infrasible-agent` mode `0640`. Generated RPMs remain binary-only (no unit or config).

## Validate data collection and management

```sh
sudo journalctl -u infrasible-agent -n 50 --no-pager
sudo -u infrasible-agent /usr/bin/infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
```

The storage and health commands require an **existing** database. Starting the daemon initializes the SQLite database and stable host Agent ID. Operator status does not create a new identity. For initial minute and quarter-hour summaries, allow the documented grace period; do not interpret early empty aggregates as a failed installation.

For correct mTLS setup, [Configuration](CONFIGURATION.md). For safe maintenance and logs, [Operations](DAEMON_RUNBOOK.md). Do not delete or copy a live database file without also accounting for its WAL and SHM.

## Integrating with Monitoring Desktop and Relay

After installing and verifying local collection, follow the [Desktop-to-Agent configuration and enrollment workflow](CONFIGURATION.md#desktop--agent--monitoring-relay-operator-integration-v030) before adding a real Monitoring server. The current Agent cannot self-enroll or automatically issue/rotate certificates. URLs in sample config are placeholders, not deployable endpoints. For central Relay implementers, distinguish the [v0.3.0/v0.3.1 executable wire contract](design/AGENT_PROTOCOL_AND_SECURITY.md#12-v030-executable-wire-vs-target-protocol--integration-handoff) from the future negotiated protocol: they are **not yet interoperable without contract alignment**. Do not claim end-to-end Production readiness merely because local installation works.
