# CI / CD — Infrasible Agent

**Daily work:** normal `git add` / `git commit` / `git push`. Agent CI verifies on GitHub. That does **not** publish a Release.

**New release:** use `scripts/ci-cd.sh` only at ship time.

---

## 1. Every day (you)

```sh
git add …
git commit -m "Describe your change."
git push
```

Optional before commit if you want a local safety net:

```sh
bash scripts/ci-cd.sh check
```

---

## 2. Before a release — see versions

```sh
bash scripts/ci-cd.sh last
```

Example output:

```text
version.go (next release must match this after you bump it):  0.3.1
latest git tag:                                               v0.3.1
latest GitHub Release:                                        v0.3.1
```

Then bump `version.go` (and `version_test.go` if needed) to the next semver, commit, push, and wait until Agent CI is green on `main`.

```sh
# edit version.go → agentVersion = "0.3.2"
git add version.go version_test.go
git commit -m "chore(release): prepare Infrasible Agent v0.3.2"
git push
gh run watch
```

---

## 3. Publish the Release

```sh
bash scripts/ci-cd.sh release 0.3.2
```

(Use the same number as `version.go`.) Confirms with `y/N`, creates tag `v0.3.2`, pushes it; **Agent Release** builds packages and publishes the GitHub Release.

```sh
gh release view v0.3.2
```

---

## What not to do

- Do not commit `dist/` (gitignored; CI builds packages).
- Do not expect every push to `main` to create a Release.
- Do not reuse an existing `vX.Y.Z` tag.

---

## GitHub workflows

| Workflow | When | Publishes Release? |
|---|---|---|
| [Agent CI](../.github/workflows/ci.yml) | every push / PR | No |
| [Agent Release](../.github/workflows/release.yml) | tag `vX.Y.Z` or manual | Yes |

Staging install: [INSTALLATION.md](INSTALLATION.md).
