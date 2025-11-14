#!/usr/bin/env bash
set -euo pipefail

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

echo "[run-local] starting Movies API on port ${PORT:-8080}"
exec go run ./cmd/server
