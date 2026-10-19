# Running tests

From the repository root.

## Full suite (local)

```sh
go test ./...
```

Runs every package once.

## CI-equivalent

```sh
go test -race -count=1 ./...
```

Matches [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) (race detector, no cached results).

## Full local gate (fmt + tidy + vet + race tests)

```sh
bash scripts/ci-cd.sh check
```

Same checks used before a version bump. See [CI_CD.md](CI_CD.md).

If Agent CI fails on `gofmt`, run `gofmt -w .` and confirm `gofmt -l .` prints nothing before pushing.
