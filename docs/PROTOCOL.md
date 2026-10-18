# Legacy protocol — v0.1.0 AS-IS ONLY

**Not the contract for the owner-approved daemon-only architecture.** This document records the shipped stdout/SSH protocol v1 for source provenance and collector reference only; the new daemon deliberately removes these public CLI commands. Read [approved target wire/security contract](design/AGENT_PROTOCOL_AND_SECURITY.md) and [product decisions](design/AGENT_PRODUCT_REQUIREMENTS.md). Do **not** silently replace a binary serving a legacy Desktop v1 consumer.

# Infrasible Agent protocol v1.0

The agent is Linux-only and emits newline-delimited JSON on stdout. Diagnostic messages go to stderr.

Commands:

- `infrasible-agent version --json`: a single JSON object with `agent_version`, `protocol_major`, `protocol_minor`, `schema_version`, `os`, `arch`, and `capabilities`.
- `infrasible-agent snapshot --json`: one JSON snapshot after approximately one second of CPU sampling.
- `infrasible-agent stream --json`: successive JSON snapshots, approximately one per second, until terminated.

Snapshot schema v1.0 contains `schema_version`, `agent_version`, `collected_at`, `interval_ms`, `host` and `errors`. Host includes CPU, memory, load, filesystems and network interface counters. Counter deltas and bytes/sec calculations are performed by the client.

The process never opens a listener, requests administrator privileges, modifies SSH configuration or reads application secrets. The client is responsible for SSH transport, server identity verification, provisioning, and lifecycle management.

Version `0.1.0` is the extracted baseline. This repository does not yet implement remote auto-update, signed release manifests, or a standalone desktop client. SHA-256 sidecars detect accidental corruption but are not an authenticity signature.
