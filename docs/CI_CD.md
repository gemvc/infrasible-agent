# CI / CD — Infrasible Agent

| Where | When | What |
|---|---|---|
| `scripts/ci-cd.sh` | daily / ship | check, version, release |
| **Agent CI** | every push / PR | `gofmt`, tidy, vet, race tests |
| **Agent Release** | tag `vX.Y.Z` only | Linux packages + GitHub Release |

Push to `main` never publishes a Release.

**Single version source:** [`VERSION.md`](../VERSION.md) (one line `X.Y.Z`). `bash scripts/ci-cd.sh version X.Y.Z` writes that file and syncs `version.go` for the binary. Do not put the number in other markdown. Doc map: [DOCUMENTATION.md](DOCUMENTATION.md). Ship verdict: [STATUS.md](STATUS.md) (edit only when capabilities change).

## Everyday

```sh
git add … && git commit -m "…" && git push
bash scripts/ci-cd.sh watch   # optional
bash scripts/ci-cd.sh check   # optional local gate (gofmt, tidy, vet, race tests)
```

Local test commands only: [TEST.md](TEST.md). Always run `gofmt -w .` (or `bash scripts/ci-cd.sh check`) before push — Agent CI fails closed on formatting.

## Ship a version

```sh
scripts/ci-cd.sh watch   # optional
scripts/ci-cd.sh check   # optional local gate (gofmt, tidy, vet, 
scripts/ci-cd.sh last
scripts/ci-cd.sh version X.Y.Z   # write VERSION.md + sync version.go, commit, push, watch Agent CI
scripts/ci-cd.sh release X.Y.Z   # tag vX.Y.Z, watch Agent Release
gh release view vX.Y.Z
```

Do not reuse an existing `vX.Y.Z` tag. Do not commit `dist/` (gitignored). Local `.deb` without tagging: `bash scripts/package-linux.sh`. Staging install: [INSTALLATION.md](INSTALLATION.md).
