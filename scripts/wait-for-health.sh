#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
MAX_RETRIES=30
SLEEP=2

for i in $(seq 1 $MAX_RETRIES); do
  if curl -fsS "${BASE_URL}/healthz" >/dev/null; then
    echo "Service healthy at ${BASE_URL}"
    exit 0
  fi
  echo "Waiting for service... (${i}/${MAX_RETRIES})"
  sleep $SLEEP
done

echo "Service did not become healthy in time" >&2
exit 1
