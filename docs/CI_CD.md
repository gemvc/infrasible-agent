# CI / CD — Infrasible Agent

**Policy:** every push and pull request is **verified** by CI. **Releases are never published** from an ordinary push to `main`. Shipping is an explicit tag / manual release action only.

| Stage | When | Workflow | Publishes? |
|---|---|---|---|
| Local verify | Before you push | [`scripts/ci-local.sh`](../scripts/ci-local.sh) | No |
| Continuous Integration | `push`/`pull_request` on `main`, or manual | [Agent CI](../.github/workflows/ci.yml) | No (CI artifacts only) |
| Continuous Delivery | Tag `vX.Y.Z` **or** Actions → Agent Release | [Agent Release](../.github/workflows/release.yml) | Yes — GitHub Release + packages |

Staging host install from CI artifacts or a Release is documented in [INSTALLATION.md](INSTALLATION.md). That is operator deploy, not this pipeline’s job.

## 1. Local gate (do this first)

```sh
bash scripts/ci-local.sh
# optional cross-build (Linux tools / dpkg may be partial on macOS):
bash scripts/package-linux.sh
```

Equivalent manual steps:

```sh
gofmt -w .          # fix formatting; CI fails if any file is dirty
test -z "$(gofmt -l .)"
go mod tidy && git diff --exit-code -- go.mod go.sum
go vet ./...
go test -race -count=1 ./...
```

A common CI failure is **gofmt** while `go test` still passes locally. Always format before push.

## 2. Continuous Integration (Agent CI)

Triggers: push to `main`, pull requests, `workflow_dispatch`.

Jobs:

1. **`verify`** — `gofmt -l .` (prints offenders on failure), `go mod tidy` drift check, `go vet`, `go test -race`.
2. **`package`** (after verify) — `scripts/package-linux.sh`, checksum check, deb/rpm metadata, short daemon CLI smoke, upload `linux-packages` artifact (14-day retention).

CI artifacts are for **staging/testing**. They are not a signed production Release. Green CI alone does not authorize production Monitoring deploy.

## 3. Continuous Delivery (Agent Release)

Does **not** run on normal commits. Two equivalent ways to ship:

### A. Manual (recommended for controlled releases)

1. Ensure `main` is green on Agent CI.
2. Commit the intended `agentVersion` in [`version.go`](../version.go) on `main` (e.g. `0.3.2`).
3. GitHub → Actions → **Agent Release** → Run workflow on `main` → input version `0.3.2` (must match `version.go`).
4. Workflow tests, packages, creates tag `v0.3.2` if missing, publishes GitHub Release assets.

### B. Tag push

```sh
# main HEAD must already have version.go == 0.3.2 and be pushed
git tag v0.3.2
git push origin v0.3.2
```

The release workflow runs on tag `vX.Y.Z`, checks the tag matches `version.go`, rebuilds packages, and publishes. It **refuses** to overwrite an existing GitHub Release of the same name.

### What is not automated

- Production host install / `systemctl` on customer machines  
- Signing / provenance beyond SHA-256 in the artifact set  
- Auto-bump of `version.go`  
- Deploy on every merge to `main`

## 4. Recommended developer loop

```text
edit → bash scripts/ci-local.sh → commit → push / PR
         ↓
    Agent CI (verify + package artifacts)
         ↓
    (optional) install CI artifact on staging host — INSTALLATION.md
         ↓
    when ready to ship: bump version.go → merge main → Agent Release or git tag vX.Y.Z
```

## 5. Troubleshooting

| Symptom | Fix |
|---|---|
| CI “Verify Go formatting” failed | `gofmt -w .` and commit |
| `go mod tidy` diff in CI | run `go mod tidy` locally; commit `go.mod`/`go.sum` |
| Release “version must match version.go” | bump `version.go` on main first, or pass the matching input |
| Release “already exists” | do not republish; bump version for a new release |
| Tests pass locally, fail in CI | reproduce with `bash scripts/ci-local.sh`; check race/Linux-only paths |

## 6. Related docs

- [INSTALLATION.md](INSTALLATION.md) — obtain artifacts and install on a staging host  
- [DAEMON_RUNBOOK.md](DAEMON_RUNBOOK.md) — operate the daemon after install  
- [AGENT_LIVE_HELLO_HEARTBEAT.md](design/AGENT_LIVE_HELLO_HEARTBEAT.md) — next protocol slice (independent of CI/CD)
