# Infrasible Monitoring Agent

Lightweight, open-source Linux host monitoring agent. The agent writes read-only CPU, memory, load, filesystem and network metrics as JSON for SSH-based clients such as Infrasible.

**Status:** released legacy baseline `v0.1.0` (published 2026-10-17). **Current license:** MIT (see LICENSE). **Approved target:** a separate daemon-only v2 design, not yet implemented; Apache-2.0 is conditional on legal review.

## Supported platforms

Linux AMD64 and ARM64; Go 1.23+ for building. No third-party Go dependencies.

## Build and test

```sh
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o infrasible-agent-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o infrasible-agent-linux-arm64 .
```

## Usage on Linux

```sh
./infrasible-agent-linux-amd64 version --json
./infrasible-agent-linux-amd64 snapshot --json
./infrasible-agent-linux-amd64 stream --json
```

`stream` continues until the client terminates the process. Each line contains a JSON snapshot. The agent does not expose a port, run as a service or configure SSH. Installation and SSH session management belong to the client.

See [legacy protocol documentation](docs/PROTOCOL.md) for the **v0.1.0** JSON contract. The **new Agent design is daemon-only** and will remove the public `version/snapshot/stream --json` commands after an explicit Desktop consumer migration; do not replace a currently deployed v1 binary silently.

## Releases

GitHub Actions checks tests and builds both architectures. A tag such as `v0.1.0` matching the version constant initiates a GitHub Release containing Linux executables and simple SHA-256 sidecars. A plain checksum **does not provide publisher authenticity**; cryptographic signing is planned before automatic network updates.

## Approved target architecture (design only)

The owner-approved architecture/DEC-01–11 and pending evidence gates live under [docs/design/AGENT_PRODUCT_REQUIREMENTS.md](docs/design/AGENT_PRODUCT_REQUIREMENTS.md), [AGENT_ARCHITECTURE.md](docs/design/AGENT_ARCHITECTURE.md), [AGENT_SQLITE_SCHEMA.md](docs/design/AGENT_SQLITE_SCHEMA.md), [AGENT_PROTOCOL_AND_SECURITY.md](docs/design/AGENT_PROTOCOL_AND_SECURITY.md), and [AGENT_ACCEPTANCE_AND_TEST_MATRIX.md](docs/design/AGENT_ACCEPTANCE_AND_TEST_MATRIX.md). [AGENT_IMPLEMENTATION_BACKLOG.md](docs/design/AGENT_IMPLEMENTATION_BACKLOG.md) is the single phased implementation backlog. **G00 is not passed; no daemon implementation, live external contract or automatic deployment is implied by docs.**

## Relationship with Infrasible

This repository contains **only** the standalone Linux Agent. It does not include the proprietary Infrasible desktop application, local SSH credential vault, inventory, or UI. Changes here do not alter the existing SSH connection implementation.
