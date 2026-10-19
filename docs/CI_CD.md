# CI / CD — Infrasible Agent

**Daily work:** normal `git add` / `git commit` / `git push`. Agent CI verifies. That does **not** publish a Release.

**New release:** use `scripts/ci-cd.sh`.

---

## 1. Every day (you)

```sh
git add …
git commit -m "Describe your change."
git push
```

---

## 2. See versions

```sh
bash scripts/ci-cd.sh last
```

---

## 3. Bump version (script)

```sh
bash scripts/ci-cd.sh version 0.3.2
```

Updates `version.go` + `version_test.go`, runs local tests, commits:

`chore(release): prepare Infrasible Agent v0.3.2`

Then **you** push and wait for green CI:

```sh
git push
gh run watch
```

---

## 4. Publish Release (script)

```sh
bash scripts/ci-cd.sh release 0.3.2
```

Same number as in step 3. Creates tag `v0.3.2`, pushes it; Agent Release publishes packages.

```sh
gh release view v0.3.2
```

---

## Full ship sequence

```sh
bash scripts/ci-cd.sh last
bash scripts/ci-cd.sh version 0.3.2
git push
gh run watch
bash scripts/ci-cd.sh release 0.3.2
```

---

## Notes

- Do not commit `dist/` (gitignored).
- Push to `main` never creates a Release by itself.
- Do not reuse an existing `vX.Y.Z` tag.

| Workflow | When | Publishes Release? |
|---|---|---|
| [Agent CI](../.github/workflows/ci.yml) | every push / PR | No |
| [Agent Release](../.github/workflows/release.yml) | tag `vX.Y.Z` | Yes |

Staging install: [INSTALLATION.md](INSTALLATION.md).
