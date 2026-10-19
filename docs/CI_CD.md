# CI / CD — Infrasible Agent

**Policy:** push to `main` only **verifies**. A **GitHub Release** is created only when you ship a version tag (or run Agent Release manually). Ordinary commits never deploy.

Replace `0.3.2` with your next semver everywhere below. Run every command from the **repository root**, one after another.

---

## Path A — Daily work (verify + push; no release)

### 1. Format

```sh
gofmt -w .
```

### 2. Confirm nothing left unformatted

```sh
test -z "$(gofmt -l .)"
```

### 3. Sync modules

```sh
go mod tidy
```

### 4. Fail if `go.mod` / `go.sum` still dirty

```sh
git diff --exit-code -- go.mod go.sum
```

### 5. Vet

```sh
go vet ./...
```

### 6. Race tests

```sh
go test -race -count=1 ./...
```

### 7. (Optional) one-shot instead of steps 1–6

```sh
bash scripts/ci-local.sh
```

### 8. (Optional) local Linux packages

```sh
bash scripts/package-linux.sh
```

### 9. Stage your changes

```sh
git status
```

### 10. Commit (edit the message)

```sh
git add -A
git commit -m "Describe your change."
```

### 11. Push (CI runs on GitHub)

```sh
git push origin HEAD
```

### 12. Watch CI (optional)

```sh
gh run watch
```

Or open: https://github.com/gemvc/infrasible-agent/actions/workflows/ci.yml  

Green CI = verified + `linux-packages` artifact. **Not** a Release.

---

## Path B — Ship a Release (tag = deploy artifacts to GitHub Releases)

Do Path A on `main` first until Agent CI is green. Then:

### 1. On latest main

```sh
git checkout main
git pull origin main
```

### 2. Set the version in source (edit the number)

```sh
# Example: set agentVersion to 0.3.2 inside version.go, then:
grep 'agentVersion' version.go
```

Confirm it prints `"0.3.2"` (or your target).

### 3. Local verify again after the version bump

```sh
bash scripts/ci-local.sh
```

### 4. Commit the version bump

```sh
git add version.go version_test.go
git commit -m "chore(release): prepare Infrasible Agent v0.3.2"
```

### 5. Push main (CI must pass)

```sh
git push origin main
```

### 6. Wait until CI is green

```sh
gh run list --workflow=ci.yml --branch main --limit 1
gh run watch
```

### 7a. Publish by tag (copy-paste release)

```sh
git tag v0.3.2
git push origin v0.3.2
```

Agent Release starts from the tag push, rebuilds packages, and creates the GitHub Release.

### 7b. Or publish from the Actions UI

```sh
gh workflow run "Agent Release" --ref main -f version=0.3.2
gh run watch
```

(`version` must match `version.go` exactly.)

### 8. Confirm the Release exists

```sh
gh release view v0.3.2
```

### 9. Download assets (optional)

```sh
gh release download v0.3.2 --dir dist-release
ls dist-release
```

Install on a staging host next: [INSTALLATION.md](INSTALLATION.md).

---

## Path C — Staging install from CI artifact (no Release yet)

Use when you want a test host before tagging.

### 1. Pick the latest successful CI run on main

```sh
gh run list --workflow=ci.yml --branch main --status success --limit 1
```

### 2. Download `linux-packages` (replace RUN_ID)

```sh
gh run download RUN_ID -n linux-packages --dir dist-ci
ls dist-ci
```

### 3. On the Linux host — install the matching deb

```sh
sudo apt install ./infrasible-agent_0.3.2_amd64.deb
sudo /usr/bin/infrasible-agent config validate --config=/etc/infrasible-agent/config.json
sudo systemctl enable --now infrasible-agent
sudo systemctl status infrasible-agent --no-pager
```

(Use `_arm64.deb` on arm64. Full notes: [INSTALLATION.md](INSTALLATION.md).)

---

## What each GitHub workflow does

| Workflow | Trigger | Publishes Release? |
|---|---|---|
| [Agent CI](../.github/workflows/ci.yml) | push / PR / manual | No — artifact only |
| [Agent Release](../.github/workflows/release.yml) | tag `vX.Y.Z` or manual + version | Yes |

---

## If something fails

```sh
gofmt -w .
bash scripts/ci-local.sh
```

```sh
go mod tidy
git add go.mod go.sum
git commit -m "chore: sync go.mod"
```

```sh
# Release already exists — bump version.go and ship a new tag, do not reuse
gh release view v0.3.2
```

---

## Related

- [INSTALLATION.md](INSTALLATION.md)  
- [DAEMON_RUNBOOK.md](DAEMON_RUNBOOK.md)  
- [AGENT_LIVE_HELLO_HEARTBEAT.md](design/AGENT_LIVE_HELLO_HEARTBEAT.md)
