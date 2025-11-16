#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${PROJECT_ROOT}/.env"

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
MAX_RETRIES="${HEALTH_MAX_RETRIES:-30}"
SLEEP="${HEALTH_SLEEP_SECONDS:-2}"

last_error=""
for i in $(seq 1 "${MAX_RETRIES}"); do
  if output=$(curl -fsS "${BASE_URL}/healthz" 2>&1); then
    echo "Service healthy at ${BASE_URL}"
    exit 0
  fi
  last_error="${output}"
  echo "Waiting for service... (${i}/${MAX_RETRIES})"
  sleep "${SLEEP}"
done

echo "Service did not become healthy in time" >&2
if [[ -n "${last_error}" ]]; then
  echo "Last error: ${last_error}" >&2
fi
exit 1
