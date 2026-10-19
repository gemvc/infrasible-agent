# CI / CD — Infrasible Agent

**Policy:** `git push` only **verifies** on GitHub. A **GitHub Release** is published only when you push tag `vX.Y.Z` (or run Agent Release manually).

**One script to remember:**

```sh
bash scripts/ci-cd.sh help
```

---

## Everyday (copy-paste)

### 1. Local gate (always)

```sh
bash scripts/ci-cd.sh check
```

This runs: guardrails (no tracked/staged `dist/`) → `gofmt -w` → tidy → vet → `go test -race`.

### 2. Commit your source changes

```sh
git status
git add -u
git add docs .github scripts .gitignore 2>/dev/null || true
git status
git commit -m "Describe your change."
```

Do **not** add `dist/`. The script refuses push if packages are staged.

### 3. Push (asks for confirmation)

```sh
bash scripts/ci-cd.sh push
```

### 4. Watch GitHub CI

```sh
bash scripts/ci-cd.sh watch
```

Green CI = tests + Linux packages artifact. **Not** a Release.

---

## Optional local packages

```sh
bash scripts/ci-cd.sh package
```

Builds under `dist/` (gitignored). Useful before a staging install from your laptop; CI still builds on Ubuntu for the official artifact.

---

## Ship a Release (tag = publish)

1. Bump `agentVersion` in `version.go`, commit, push, wait for green CI.
2. Then:

```sh
bash scripts/ci-cd.sh release 0.3.2
```

(Use your real semver; must match `version.go`.) Creates `v0.3.2`, pushes the tag, Agent Release publishes assets.

Or without the script:

```sh
git tag v0.3.2
git push origin v0.3.2
```

---

## Staging install from CI artifact

```sh
gh run list --workflow=ci.yml --branch main --status success --limit 1
gh run download RUN_ID -n linux-packages --dir dist-ci
ls dist-ci
```

Then on the Linux host (see [INSTALLATION.md](INSTALLATION.md)):

```sh
sudo apt install ./infrasible-agent_0.3.1_amd64.deb
sudo /usr/bin/infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl enable --now infrasible-agent
```

---

## What the script prevents

| Mistake | Guard |
|---|---|
| Forgetting `gofmt` | `check` formats and fails if still dirty |
| Forgetting tidy | fails if `go.mod`/`go.sum` drift |
| Committing `dist/` or `*.sha256` | refused if tracked or staged |
| Pushing with uncommitted files | `push` requires clean tree |
| Release version mismatch | `release X.Y.Z` must equal `version.go` |
| Accidental push/release | interactive `y/N` confirm |

---

## GitHub workflows

| Workflow | Trigger | Publishes Release? |
|---|---|---|
| [Agent CI](../.github/workflows/ci.yml) | push / PR | No |
| [Agent Release](../.github/workflows/release.yml) | tag `vX.Y.Z` or manual | Yes |

---

## If CI fails

```sh
bash scripts/ci-cd.sh check
bash scripts/ci-cd.sh package
```

| Symptom | Fix |
|---|---|
| gofmt | already fixed by `check` — commit the reformatted files |
| dist checksum / `.sha256` | fixed in `package-linux.sh`; never commit `dist/` |
| Go cache `tar` annotation | ignore — not a real failure |
| Release exists | bump version; do not reuse the tag |

---

## Related

- [INSTALLATION.md](INSTALLATION.md)  
- [DAEMON_RUNBOOK.md](DAEMON_RUNBOOK.md)  
- [AGENT_LIVE_HELLO_HEARTBEAT.md](design/AGENT_LIVE_HELLO_HEARTBEAT.md)
