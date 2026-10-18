# Legacy v0.1.0 installation (not daemon-v2 instructions)

These instructions describe the currently released **CLI-based v0.1.0** artifact. The new approved design is a separate **daemon-only service**, which has not been implemented or released. Protect existing SSH/CLI consumers with a versioned migration; see [P00 requirements](design/AGENT_PRODUCT_REQUIREMENTS.md) and [backlog](design/AGENT_IMPLEMENTATION_BACKLOG.md). The MIT-to-Apache-2.0 intention is legally conditional; current releases remain MIT.

# Linux builds and installation

Release packages support:
- Ubuntu, Debian, Raspberry Pi OS (64-bit): `linux-arm64` or `.deb arm64` on ARM64.
- Raspberry Pi OS (32-bit ARMv7, including many Pi 3/4 installations): `linux-armv7` or `.deb armhf`.
- Older Raspberry Pi ARMv6 (Pi 1 / Zero): `linux-armv6` tarball (no Debian package in this release).
- Linux x86_64: `linux-amd64`, `.deb amd64` and `.rpm x86_64`.
- Fedora, RHEL, Rocky and AlmaLinux ARM64, ARMv7: use matching generic Linux tarball for now.

Check target with `uname -m`. ARM OS bitness, not Raspberry Pi hardware capability, determines the correct build.

The archive contains `infrasible-agent` and `LICENSE`. Example:
```sh
tar -xzf infrasible-agent_0.1.0_linux_arm64.tar.gz
chmod +x infrasible-agent
./infrasible-agent version --json
./infrasible-agent snapshot --json
```

The agent makes no outbound connection and has no SSH credentials, background service, daemon or HTTP port. Infrasible Desktop manages the SSH session and user-local agent lifecycle separately. Avoid package installation on hosts already managed by the Desktop until paths and ownership are explicitly reconciled.

For integrity checks, download `SHA256SUMS` and verify with `sha256sum -c SHA256SUMS`. **SHA-256 without a trusted signature does not authenticate a release.** Validate the publisher and trusted GitHub release origin independently. No automatic updates are enabled.

Releases are created only from a `vX.Y.Z` tag matching `agentVersion` in `version.go`. The CI workflow tests each PR, and the release workflow re-runs tests before publishing artifacts. No distribution-specific OS runtime qualification or ARM hardware execution tests have yet been performed; cross-build does not prove runtime compatibility.
