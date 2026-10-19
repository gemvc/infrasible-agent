#!/usr/bin/env bash
# Release helper. Daily work stays normal git add / commit / push.
#
#   bash scripts/ci-cd.sh last              # show current + latest released version
#   bash scripts/ci-cd.sh release X.Y.Z     # verify, tag vX.Y.Z, push tag → GitHub Release
#   bash scripts/ci-cd.sh check             # optional local gate (fmt/tidy/vet/test)
#   bash scripts/ci-cd.sh help
#
# See docs/CI_CD.md
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CMD="${1:-help}"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }
ok() { echo "OK: $*"; }

agent_version() {
  sed -n 's/^const agentVersion = "\([^"]*\)"/\1/p' version.go
}

latest_tag() {
  git fetch --tags --quiet origin 2>/dev/null || true
  git tag -l 'v[0-9]*' --sort=-v:refname | head -1
}

confirm() {
  local prompt="$1"
  local reply
  read -r -p "$prompt [y/N] " reply
  [[ "$reply" == "y" || "$reply" == "Y" ]]
}

guardrails() {
  if [[ ! -f .gitignore ]] || ! grep -q '^/dist/' .gitignore; then
    die ".gitignore must ignore /dist/"
  fi
  if git ls-files 'dist/*' 2>/dev/null | grep -q .; then
    die "dist/ is tracked by git — run: git rm -r --cached dist/"
  fi
  if git diff --cached --name-only | grep -E '(^|/)dist/|\.deb$|\.rpm$|\.sha256$' >/dev/null; then
    die "staged dist/ or package files — unstage them (CI builds artifacts)"
  fi
}

fmt_tidy_vet_test() {
  info "gofmt -w ."
  gofmt -w .
  local unformatted
  unformatted="$(gofmt -l .)"
  [[ -z "$unformatted" ]] || { echo "$unformatted"; die "gofmt left files dirty"; }

  info "go mod tidy"
  go mod tidy
  git diff --exit-code -- go.mod go.sum >/dev/null || {
    git diff -- go.mod go.sum || true
    die "go.mod/go.sum changed — commit them on main before release"
  }

  info "go vet ./..."
  go vet ./...

  info "go test -race -count=1 ./..."
  go test -race -count=1 ./...
}

cmd_last() {
  local source_ver tag gh_ver
  source_ver="$(agent_version)"
  tag="$(latest_tag)"
  gh_ver=""
  if command -v gh >/dev/null 2>&1; then
    gh_ver="$(gh release view --json tagName -q .tagName 2>/dev/null || true)"
  fi

  echo "version.go (next release must match this after you bump it):  $source_ver"
  if [[ -n "$tag" ]]; then
    echo "latest git tag:                                               $tag"
  else
    echo "latest git tag:                                               (none)"
  fi
  if [[ -n "$gh_ver" ]]; then
    echo "latest GitHub Release:                                        $gh_ver"
  else
    echo "latest GitHub Release:                                        (none or gh unavailable)"
  fi
  echo
  echo "Daily work:  git add / commit / push as usual (Agent CI verifies)."
  echo "To ship:     1) bump version.go → commit → push → wait for green CI"
  echo "             2) bash scripts/ci-cd.sh release $source_ver"
}

cmd_check() {
  guardrails
  fmt_tidy_vet_test
  ok "local verify passed"
}

cmd_release() {
  local ver="${2:-}"
  [[ -n "$ver" ]] || die "usage: bash scripts/ci-cd.sh release X.Y.Z"
  [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be semver X.Y.Z (got: $ver)"

  guardrails
  fmt_tidy_vet_test

  local source_ver
  source_ver="$(agent_version)"
  [[ "$ver" == "$source_ver" ]] || die "version.go is $source_ver but you passed $ver — bump and commit version.go first"

  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "main" ]] || die "release only from main (currently: $branch)"

  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "working tree dirty — commit everything on main first"
  fi

  git fetch origin main
  [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || \
    die "main != origin/main — git push (or pull) first, wait for green CI"

  if git ls-remote --exit-code --refs origin "refs/tags/v$ver" >/dev/null 2>&1; then
    die "origin already has tag v$ver"
  fi

  echo
  echo "Ship GitHub Release v$ver from $(git rev-parse --short HEAD) on main."
  if ! confirm "Create and push tag v$ver now?"; then
    die "release cancelled"
  fi

  git tag "v$ver"
  git push origin "v$ver"
  ok "tag v$ver pushed — Agent Release publishes packages"
  echo "Watch:  gh run watch"
  echo "View:   gh release view v$ver"
}

cmd_help() {
  cat <<'EOF'
scripts/ci-cd.sh

Daily (you do this yourself):
  git add …
  git commit -m "…"
  git push

When you want a new release:
  bash scripts/ci-cd.sh last              # see current / latest versions
  # bump version.go, commit, push, wait for green Agent CI
  bash scripts/ci-cd.sh release X.Y.Z     # tag + publish Release

Optional:
  bash scripts/ci-cd.sh check             # fmt + tidy + vet + race tests
  bash scripts/ci-cd.sh help
EOF
}

case "$CMD" in
  last) cmd_last ;;
  check) cmd_check ;;
  release) cmd_release "$@" ;;
  help|-h|--help) cmd_help ;;
  *) die "unknown: $CMD (try: bash scripts/ci-cd.sh help)" ;;
esac
