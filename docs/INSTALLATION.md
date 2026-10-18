# Linux installation — Infrasible Agent v0.2.0

> **Pre-production testing only.** v0.2.0 is an implementation candidate. It can collect and aggregate locally with no monitoring endpoints; it is not a fully integrated central monitoring deployment.

## Download a built artifact

Use the latest **successful main** [Agent CI run](https://github.com/gemvc/infrasible-agent/actions/workflows/ci.yml), open its **Artifacts** section and download `linux-packages` ZIP. A successful CI run produces the native Linux executables, tarballs, Debian packages on amd64/arm64/armhf, an x86_64 RPM, `SHA256SUMS` and per-binary `.sha256` files. **CI artifacts are not automatically a GitHub Release.**

When a manually approved `v0.2.0` release has been published, its verified assets can instead be obtained from [Releases](https://github.com/gemvc/infrasible-agent/releases). Never install the old `v0.1.0` asset expecting daemon-v2 functionality.

Match the Linux OS architecture using `uname -m` and `dpkg --print-architecture`. Use `infrasible-agent_0.2.0_amd64.deb` on Debian/Ubuntu amd64 or `infrasible-agent_0.2.0_arm64.deb` on arm64. On other Linux targets use the matching tarball; generated RPMs only package the binary, not systemd/configuration.

Verify checksums for the downloaded matching package from `SHA256SUMS` when present. SHA-256 alone does **not** authenticate the publisher or protect against a compromised CI account.

## Install on Ubuntu/Debian test host

After downloading/extracting artifacts to the Linux machine:

```sh
sudo apt install ./infrasible-agent_0.2.0_amd64.deb
sudo useradd --system --home-dir /var/lib/infrasible-agent --shell /usr/sbin/nologin infrasible-agent
sudo install -d -m 0750 -o infrasible-agent -g infrasible-agent /var/lib/infrasible-agent
sudo install -d -m 0750 /etc/infrasible-agent
sudo install -m 0640 -o root -g infrasible-agent \
  /usr/share/doc/infrasible-agent/config.example.json \
  /etc/infrasible-agent/config.json
sudo systemctl daemon-reload
sudo /usr/bin/infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl enable --now infrasible-agent
sudo systemctl status infrasible-agent --no-pager
```

The sample config specifies an empty monitoring server list. It does **not** make the Agent a connected or registered production monitor. Package installation does **not** create the service user, install production certificates, enroll the agent, enable or start the service automatically.

For subsequent installs use `id infrasible-agent` before attempting to create an existing user. For tarballs, install the binary to `/usr/bin/infrasible-agent`, its `infrasible-agent.service` to `/etc/systemd/system/infrasible-agent.service`, and its `config.example.json` to `/etc/infrasible-agent/config.json` with the same ownership rules.

## Validate data collection and management

```sh
sudo journalctl -u infrasible-agent -n 50 --no-pager
sudo -u infrasible-agent /usr/bin/infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage cleanup --dry-run --config=/etc/infrasible-agent/config.json
```

The storage and health commands require an **existing** database. Starting the daemon initializes the SQLite database and stable host Agent ID. Operator status does not create a new identity. For initial minute and quarter-hour summaries, allow the documented grace period; do not interpret early empty aggregates as a failed installation.

For correct mTLS setup, [Configuration](CONFIGURATION.md). For safe maintenance and logs, [Operations](DAEMON_RUNBOOK.md). Do not delete or copy a live database file without also accounting for its WAL and SHM.
