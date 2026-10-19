#!/usr/bin/env bash
# Optional local gate — alias for: bash scripts/ci-cd.sh check
set -euo pipefail
exec "$(cd "$(dirname "$0")" && pwd)/ci-cd.sh" check
