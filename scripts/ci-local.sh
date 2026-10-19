#!/usr/bin/env bash
# Mirror Agent CI verify steps on a developer machine. Run before every push.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> gofmt"
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "Unformatted files (run: gofmt -w .):"
  echo "$unformatted"
  exit 1
fi

echo "==> go mod tidy"
go mod tidy
git diff --exit-code -- go.mod go.sum

echo "==> go vet"
go vet ./...

echo "==> go test -race"
go test -race -count=1 ./...

echo "Local CI verify OK. Optional package build: bash scripts/package-linux.sh"
