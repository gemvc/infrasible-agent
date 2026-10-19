#!/usr/bin/env bash
# Release helper. Daily work stays normal git add / commit / push.
#
#   bash scripts/ci-cd.sh last              # show current + latest released version
#   bash scripts/ci-cd.sh version X.Y.Z     # bump version.go, commit prepare message
#   bash scripts/ci-cd.sh watch             # gh run watch (live CI in terminal)
#   bash scripts/ci-cd.sh web               # gh run view --web (open CI in browser)
#   bash scripts/ci-cd.sh release X.Y.Z     # tag vX.Y.Z, push tag → GitHub Release
#   bash scripts/ci-cd.sh check             # optional local gate
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

need_gh() {
  command -v gh >/dev/null 2>&1 || die "gh CLI required (brew install gh && gh auth login)"
}

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

require_semver() {
  local ver="$1"
  [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be semver X.Y.Z (got: $ver)"
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
    die "go.mod/go.sum changed — commit them before continuing"
  }

  info "go vet ./..."
  go vet ./...

  info "go test -race -count=1 ./..."
  go test -race -count=1 ./...
}

set_version_files() {
  local ver="$1"
  local tmp
  [[ -n "$(agent_version)" ]] || die "could not read agentVersion from version.go"

  tmp="$(mktemp)"
  sed "s/^const agentVersion = \".*\"/const agentVersion = \"$ver\"/" version.go >"$tmp"
  mv "$tmp" version.go

  tmp="$(mktemp)"
  awk -v ver="$ver" '
    /if agentVersion !=/ { print "\tif agentVersion != \"" ver "\" {"; next }
    { print }
  ' version_test.go >"$tmp"
  mv "$tmp" version_test.go

  [[ "$(agent_version)" == "$ver" ]] || die "failed to set version.go to $ver"
  grep -q "agentVersion != \"$ver\"" version_test.go || die "failed to update version_test.go"
}

cmd_last() {
  local source_ver tag gh_ver
  source_ver="$(agent_version)"
  tag="$(latest_tag)"
  gh_ver=""
  if command -v gh >/dev/null 2>&1; then
    gh_ver="$(gh release view --json tagName -q .tagName 2>/dev/null || true)"
  fi

  echo "version.go:            $source_ver"
  echo "latest git tag:        ${tag:-"(none)"}"
  echo "latest GitHub Release: ${gh_ver:-"(none or gh unavailable)"}"
  echo
  echo "Next for a new release:"
  echo "  bash scripts/ci-cd.sh version X.Y.Z"
  echo "  git push"
  echo "  bash scripts/ci-cd.sh watch"
  echo "  bash scripts/ci-cd.sh web"
  echo "  bash scripts/ci-cd.sh release X.Y.Z"
}

cmd_check() {
  guardrails
  fmt_tidy_vet_test
  ok "local verify passed"
}

cmd_version() {
  local ver="${2:-}"
  [[ -n "$ver" ]] || die "usage: bash scripts/ci-cd.sh version X.Y.Z"
  require_semver "$ver"

  local current
  current="$(agent_version)"
  [[ "$ver" != "$current" ]] || die "version.go is already $ver"

  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "main" ]] || die "bump version on main (currently: $branch)"

  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "working tree dirty — commit or stash your daily work first"
  fi

  if git ls-remote --exit-code --refs origin "refs/tags/v$ver" >/dev/null 2>&1; then
    die "origin already has tag v$ver — pick a new version"
  fi

  echo "Bump agentVersion: $current → $ver"
  if ! confirm "Update version.go / version_test.go and commit?"; then
    die "cancelled"
  fi

  set_version_files "$ver"
  guardrails
  fmt_tidy_vet_test

  git add version.go version_test.go
  git commit -m "chore(release): prepare Infrasible Agent v$ver"
  ok "committed version bump to $ver"
  echo
  echo "You push (as usual):"
  echo "  git push"
  echo "Watch CI:"
  echo "  bash scripts/ci-cd.sh watch"
  echo "  bash scripts/ci-cd.sh web"
  echo "When green:"
  echo "  bash scripts/ci-cd.sh release $ver"
}

cmd_watch() {
  need_gh
  info "gh run watch"
  gh run watch --exit-status
}

cmd_web() {
  need_gh
  info "gh run view --web"
  gh run view --web
}

cmd_release() {
  local ver="${2:-}"
  [[ -n "$ver" ]] || die "usage: bash scripts/ci-cd.sh release X.Y.Z"
  require_semver "$ver"

  guardrails
  fmt_tidy_vet_test

  local source_ver
  source_ver="$(agent_version)"
  [[ "$ver" == "$source_ver" ]] || die "version.go is $source_ver but you passed $ver — run: bash scripts/ci-cd.sh version $ver"

  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "main" ]] || die "release only from main (currently: $branch)"

  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "working tree dirty — commit everything on main first"
  fi

  git fetch origin main
  [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || \
    die "main != origin/main — git push first, wait for green CI"

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
  echo "Watch release workflow:"
  echo "  bash scripts/ci-cd.sh watch"
  echo "  bash scripts/ci-cd.sh web"
  echo "View release: gh release view v$ver"
}

cmd_help() {
  cat <<'EOF'
scripts/ci-cd.sh

Daily (you):
  git add … && git commit -m "…" && git push

New release:
  bash scripts/ci-cd.sh last
  bash scripts/ci-cd.sh version X.Y.Z    # bump + commit prepare
  git push
  bash scripts/ci-cd.sh watch            # live CI in terminal
  bash scripts/ci-cd.sh web              # open CI in browser
  bash scripts/ci-cd.sh release X.Y.Z    # tag + GitHub Release

Optional:
  bash scripts/ci-cd.sh check
  bash scripts/ci-cd.sh help
EOF
}

case "$CMD" in
  last) cmd_last ;;
  version) cmd_version "$@" ;;
  watch) cmd_watch ;;
  web) cmd_web ;;
  check) cmd_check ;;
  release) cmd_release "$@" ;;
  help|-h|--help) cmd_help ;;
  *) die "unknown: $CMD (try: bash scripts/ci-cd.sh help)" ;;
esac
