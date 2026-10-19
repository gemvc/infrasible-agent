# CI / CD — Infrasible Agent

Local script and GitHub Actions share one model:

| Where | When | What |
|---|---|---|
| You + `scripts/ci-cd.sh` | daily / ship | check, version bump, tag |
| **Agent CI** | every push / PR | fast verify only (`gofmt`, tidy, vet, race tests) |
| **Agent Release** | tag `vX.Y.Z` only | package Linux artifacts + GitHub Release |

Push to `main` never publishes a Release. Packaging runs only on release tags (keeps daily CI fast).

---

## 1. Every day (you)

```sh
git add …
git commit -m "Describe your change."
git push
bash scripts/ci-cd.sh watch    # optional
bash scripts/ci-cd.sh web      # optional
```

Optional local gate before commit:

```sh
bash scripts/ci-cd.sh check
```

---

## 2. See versions

```sh
bash scripts/ci-cd.sh last
```

---

## 3. Bump version

```sh
bash scripts/ci-cd.sh version 0.3.2
git push
bash scripts/ci-cd.sh watch
bash scripts/ci-cd.sh web
```

---

## 4. Publish Release

```sh
bash scripts/ci-cd.sh release 0.3.2
bash scripts/ci-cd.sh watch
bash scripts/ci-cd.sh web
gh release view v0.3.2
```

`release` pushes tag `v0.3.2` → **Agent Release** builds packages and publishes.

---

## Full ship sequence

```sh
bash scripts/ci-cd.sh last
bash scripts/ci-cd.sh version 0.3.2
git push
bash scripts/ci-cd.sh watch
bash scripts/ci-cd.sh release 0.3.2
bash scripts/ci-cd.sh watch
```

---

## Notes

- Do not commit `dist/` (gitignored). Packages appear on the GitHub Release only.
- Need a `.deb` without tagging? `bash scripts/package-linux.sh` locally.
- Staging install: [INSTALLATION.md](INSTALLATION.md).
- Do not reuse an existing `vX.Y.Z` tag.
