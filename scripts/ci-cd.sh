#!/usr/bin/env bash
# Single entry point for local CI/CD. Prefer this over remembering individual commands.
#
# Usage:
#   bash scripts/ci-cd.sh              # same as: check
#   bash scripts/ci-cd.sh check        # format, tidy, vet, race tests, guardrails
#   bash scripts/ci-cd.sh package      # check + cross-build packages
#   bash scripts/ci-cd.sh push         # check, then git push (refuses unsafe state)
#   bash scripts/ci-cd.sh watch        # gh run watch (latest Agent CI)
#   bash scripts/ci-cd.sh release X.Y.Z  # tag vX.Y.Z and push (after version.go matches)
#   bash scripts/ci-cd.sh help
#
# See docs/CI_CD.md
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CMD="${1:-check}"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }
ok() { echo "OK: $*"; }

agent_version() {
  sed -n 's/^const agentVersion = "\([^"]*\)"/\1/p' version.go
}

confirm() {
  local prompt="$1"
  local reply
  read -r -p "$prompt [y/N] " reply
  [[ "$reply" == "y" || "$reply" == "Y" ]]
}

# Refuse committing or pushing build outputs / sidecar checksum pitfalls.
guardrails() {
  info "guardrails"
  if [[ ! -f .gitignore ]] || ! grep -q '^/dist/' .gitignore; then
    die ".gitignore must ignore /dist/ (see docs/CI_CD.md)"
  fi
  if git ls-files --error-unmatch dist >/dev/null 2>&1 || git ls-files 'dist/*' | grep -q .; then
    die "dist/ is still tracked by git. Run: git rm -r --cached dist/  (keep .gitignore)"
  fi
  if git diff --cached --name-only | grep -E '(^|/)dist/|\.deb$|\.rpm$' >/dev/null; then
    die "staged files include dist/ or packages — unstage them (CI builds artifacts)"
  fi
  if git diff --cached --name-only | grep -E '\.sha256$' >/dev/null; then
    die "staged *.sha256 sidecars — do not commit them"
  fi
  ok "no dist/ packages staged or tracked"
}

fmt_tidy_vet_test() {
  info "gofmt -w ."
  gofmt -w .

  info "gofmt check"
  local unformatted
  unformatted="$(gofmt -l .)"
  if [[ -n "$unformatted" ]]; then
    echo "$unformatted"
    die "gofmt left files unformatted"
  fi

  info "go mod tidy"
  go mod tidy
  if ! git diff --exit-code -- go.mod go.sum >/dev/null; then
    echo "go.mod / go.sum were updated by tidy — review and include them in your commit:"
    git diff -- go.mod go.sum || true
    die "commit go.mod/go.sum after tidy (or restore if unintended)"
  fi

  info "go vet ./..."
  go vet ./...

  info "go test -race -count=1 ./..."
  go test -race -count=1 ./...
}

cmd_check() {
  guardrails
  fmt_tidy_vet_test
  ok "local verify passed (mirrors Agent CI verify job)"
  echo "Next: commit source changes, then: bash scripts/ci-cd.sh push"
}

cmd_package() {
  cmd_check
  info "scripts/package-linux.sh"
  bash scripts/package-linux.sh
  info "verify SHA256SUMS"
  (cd dist && sha256sum -c SHA256SUMS)
  if grep -E '\.sha256$' dist/SHA256SUMS >/dev/null 2>&1; then
    die "SHA256SUMS incorrectly lists .sha256 sidecars — fix scripts/package-linux.sh"
  fi
  ok "packages built under dist/ (gitignored; do not commit)"
}

cmd_push() {
  cmd_check

  info "git status"
  git status -sb

  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "working tree has uncommitted changes — commit (source only) before push"
  fi

  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  info "pushing $branch to origin"
  if ! confirm "Push $branch to origin?"; then
    die "push cancelled"
  fi
  git push -u origin HEAD
  ok "pushed — Agent CI will verify (does not publish a Release)"
  echo "Watch: bash scripts/ci-cd.sh watch"
}

cmd_watch() {
  command -v gh >/dev/null || die "gh CLI required (brew install gh)"
  info "watching latest Agent CI run"
  gh run watch --exit-status
}

cmd_release() {
  local ver="${2:-}"
  [[ -n "$ver" ]] || die "usage: bash scripts/ci-cd.sh release X.Y.Z"
  [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be semver X.Y.Z (got: $ver)"

  cmd_check

  local source_ver
  source_ver="$(agent_version)"
  [[ "$ver" == "$source_ver" ]] || die "version.go has $source_ver but you asked to release $ver — bump version.go first and commit"

  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "main" ]] || die "release only from main (on: $branch)"

  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "working tree dirty — commit version bump on main first"
  fi

  git fetch origin main
  local head remote
  head="$(git rev-parse HEAD)"
  remote="$(git rev-parse origin/main)"
  [[ "$head" == "$remote" ]] || die "main is not in sync with origin/main — push or pull first"

  if git ls-remote --exit-code --refs origin "refs/tags/v$ver" >/dev/null 2>&1; then
    die "origin already has tag v$ver"
  fi

  echo "Will create and push tag v$ver (triggers Agent Release)."
  if ! confirm "Ship GitHub Release v$ver?"; then
    die "release cancelled"
  fi

  git tag "v$ver"
  git push origin "v$ver"
  ok "tag v$ver pushed — Agent Release should publish packages"
  echo "Check: gh release view v$ver"
  echo "Or:    gh run watch"
}

cmd_help() {
  cat <<'EOF'
scripts/ci-cd.sh — local CI/CD helper

  check              Format, tidy, vet, race tests + safety guardrails (default)
  package            check + build dist/ packages + verify SHA256SUMS
  push               check, require clean tree, confirm, git push
  watch              gh run watch (Agent CI)
  release X.Y.Z      require version.go match, tag vX.Y.Z, push tag (Release)
  help               This text

Typical day:
  bash scripts/ci-cd.sh check
  git add … && git commit -m "…"
  bash scripts/ci-cd.sh push
  bash scripts/ci-cd.sh watch

Ship:
  # version.go already committed on main as X.Y.Z and CI green
  bash scripts/ci-cd.sh release X.Y.Z

Never commit dist/. CI builds packages. Push does not publish a Release.
EOF
}

case "$CMD" in
  check) cmd_check ;;
  package) cmd_package ;;
  push) cmd_push ;;
  watch) cmd_watch ;;
  release) cmd_release "$@" ;;
  help|-h|--help) cmd_help ;;
  *) die "unknown command: $CMD (try: bash scripts/ci-cd.sh help)" ;;
esac
