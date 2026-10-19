From the repo root:


go test ./...
That runs every package once (same baseline used locally after the collector refactor).

Closer to CI (race detector, no cached results):


go test -race -count=1 ./...
That matches .github/workflows/ci.yml.

Optional local gate (fmt, tidy, vet, tests — as in docs/CI_CD.md):


bash scripts/ci-cd.sh check