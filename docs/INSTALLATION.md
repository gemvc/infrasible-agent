# Linux installation — Infrasible Agent

> Staging / pre-production. Offline collection works without central endpoints. Production Relay/PKI is not certified by install alone. See [STATUS.md](STATUS.md).

## Get a package

From a [GitHub Release](https://github.com/gemvc/infrasible-agent/releases) (`bash scripts/ci-cd.sh release X.Y.Z`), or build locally with `bash scripts/package-linux.sh`. Current semver is only in [`VERSION.md`](../VERSION.md) (`bash scripts/ci-cd.sh last`). Match arch via `uname -m` / `dpkg --print-architecture`. Artifact names look like `infrasible-agent_<VERSION>_amd64.deb` or `_arm64.deb`. Verify `SHA256SUMS` when present (integrity only, not publisher auth).

## Install (Debian/Ubuntu)

```sh
# <VERSION> and arch = the Release asset you downloaded (or scripts/package-linux.sh output)
sudo apt install ./infrasible-agent_<VERSION>_amd64.deb   # or arm64
sudo /usr/bin/infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl enable --now infrasible-agent
sudo systemctl status infrasible-agent --no-pager
```

`postinst` creates user `infrasible-agent`, data/config dirs, and installs example config (empty `monitoring_servers`) if missing. It does **not** start the service. Client private keys for Live must be mode `0600` and owned by the service user.

## Validate offline collection

```sh
sudo journalctl -u infrasible-agent -n 50 --no-pager
sudo -u infrasible-agent /usr/bin/infrasible-agent health status --config=/etc/infrasible-agent/config.json
sudo -u infrasible-agent /usr/bin/infrasible-agent storage status --config=/etc/infrasible-agent/config.json
```

Wait for DB bootstrap; early empty minute/quarter rollups are normal until UTC grace. Config, mTLS, Relay: [CONFIGURATION.md](CONFIGURATION.md), [MONITORING_RELAY.md](design/MONITORING_RELAY.md). Ops: [DAEMON_RUNBOOK.md](DAEMON_RUNBOOK.md).
