#!/usr/bin/env bash
# Deprecated alias — use scripts/ci-cd.sh check
set -euo pipefail
exec "$(cd "$(dirname "$0")" && pwd)/ci-cd.sh" check
