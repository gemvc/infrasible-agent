# CI / CD — Infrasible Agent

| Where | When | What |
|---|---|---|
| `scripts/ci-cd.sh` | daily / ship | check, version, release |
| **Agent CI** | every push / PR | `gofmt`, tidy, vet, race tests |
| **Agent Release** | tag `vX.Y.Z` only | Linux packages + GitHub Release |

Push to `main` never publishes a Release.

## Everyday

```sh
git add … && git commit -m "…" && git push
bash scripts/ci-cd.sh watch   # optional
bash scripts/ci-cd.sh check   # optional local gate
```

## Ship a version

```sh
bash scripts/ci-cd.sh last
bash scripts/ci-cd.sh version X.Y.Z   # bump, commit, push, watch Agent CI
bash scripts/ci-cd.sh release X.Y.Z   # tag vX.Y.Z, watch Agent Release
gh release view vX.Y.Z
```

Do not reuse an existing `vX.Y.Z` tag. Do not commit `dist/` (gitignored). Local `.deb` without tagging: `bash scripts/package-linux.sh`. Staging install: [INSTALLATION.md](INSTALLATION.md). Status: [STATUS.md](STATUS.md).
