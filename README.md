# Infrasible Monitoring Agent

Lightweight, open-source Linux host monitoring agent. The agent writes read-only CPU, memory, load, filesystem and network metrics as JSON for SSH-based clients such as Infrasible.

**Status:** extracted baseline `v0.1.0` (not yet released). **License:** MIT (see LICENSE).

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

See [protocol documentation](docs/PROTOCOL.md) for the v1 JSON contract.

## Releases

GitHub Actions checks tests and builds both architectures. A tag such as `v0.1.0` matching the version constant initiates a GitHub Release containing Linux executables and simple SHA-256 sidecars. A plain checksum **does not provide publisher authenticity**; cryptographic signing is planned before automatic network updates.

## Relationship with Infrasible

This repository contains **only** the standalone Linux Agent. It does not include the proprietary Infrasible desktop application, local SSH credential vault, inventory, or UI. Changes here do not alter the existing SSH connection implementation.
