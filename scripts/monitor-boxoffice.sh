#!/usr/bin/env bash
# Periodic BoxOffice upstream health monitor with schema drift detection.
# 1) 从 .env (或 ENV_FILE) 加载 BOXOFFICE_URL / BOXOFFICE_API_KEY
# 2) 跑 boxoffice 包单测（解析契约自检，不依赖真实上游）
# 3) 直接探测真实上游（TestHTTPClientSmoke）
# 4) 对比 OpenAPI 输出字段与实时返回的字段，字段缺失则告警

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG_DIR="${ROOT}/monitor-logs"
mkdir -p "$LOG_DIR"
LOG_FILE="${LOG_DIR}/boxoffice_monitor_$(date +%Y%m%d%H%M%S).log"

ENV_FILE=${ENV_FILE:-"${ROOT}/.env"}
ALERT_EMAIL=${ALERT_EMAIL:-}
SAMPLE_TITLE=${SAMPLE_TITLE:-Inception}

log() { echo "[$(date +'%F %T')] $*" | tee -a "$LOG_FILE"; }

send_alert() {
  local message=$1
  if [[ -n "$ALERT_EMAIL" ]] && command -v mail >/dev/null 2>&1; then
    printf '%s\n' "$message" | mail -s "[BoxOffice Monitor] Failure" "$ALERT_EMAIL"
  else
    log "[WARN] Alert email not sent (ALERT_EMAIL unset或 mail 未安装)"
  fi
}

load_env() {
  if [[ -f "$ENV_FILE" ]]; then
    log "[STEP] Loading env from $ENV_FILE"
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
  else
    log "[WARN] ENV file $ENV_FILE not found; relying on current environment"
  fi
  if [[ -z "${BOXOFFICE_URL:-}" || -z "${BOXOFFICE_API_KEY:-}" ]]; then
    log "[ERROR] BOXOFFICE_URL 或 BOXOFFICE_API_KEY 缺失"
    send_alert "BOXOFFICE monitor failed: missing BOXOFFICE_URL or BOXOFFICE_API_KEY"
    exit 1
  fi
}

run_tests() {
  log "[STEP] Running boxoffice unit/fuzz sanity"
  (cd "$ROOT" && go test ./internal/boxoffice -run .) | tee -a "$LOG_FILE"
}

probe_upstream() {
  log "[STEP] Probing real upstream $BOXOFFICE_URL"
  set +e
  (cd "$ROOT" && BOXOFFICE_URL="$BOXOFFICE_URL" BOXOFFICE_API_KEY="$BOXOFFICE_API_KEY" go test ./internal/boxoffice -run TestHTTPClientSmoke >/dev/null)
  local rc=$?
  set -e
  if [[ $rc -ne 0 ]]; then
    log "[ERROR] Upstream probe failed"
    send_alert "BOXOFFICE monitor failed: upstream probe error"
    exit 1
  fi
  log "[OK] Upstream probe passed"
}

schema_fields() {
  # 提取 OpenAPI 中 200 响应的字段集合（顶层和 revenue 子字段）
  jq -r '.components.schemas.BoxOfficeRecord.properties | to_entries[] | .key' "$ROOT/boxoffice.openapi.yml"
  jq -r '.components.schemas.BoxOfficeRecord.properties.revenue.properties | to_entries[] | "revenue." + .key' "$ROOT/boxoffice.openapi.yml"
}

probe_schema() {
  log "[STEP] Checking schema drift against openapi"
  local resp tmp
  tmp=$(mktemp)
  set +e
  resp=$(curl -sSf -H "X-API-Key: ${BOXOFFICE_API_KEY}" "${BOXOFFICE_URL}/boxoffice?title=${SAMPLE_TITLE}")
  rc=$?
  set -e
  if [[ $rc -ne 0 ]]; then
    log "[ERROR] Failed to fetch sample response for schema check"
    send_alert "BOXOFFICE monitor failed: cannot fetch sample for schema check"
    return
  fi
  echo "$resp" > "$tmp"
  local fields expected missing
  fields=$(jq -r 'paths(scalars) as $p | ($p | map(tostring) | join("."))' "$tmp" | awk -F'.' '{print $1}' | sort -u)
  expected=$(schema_fields | sort -u)
  missing=$(comm -23 <(echo "$expected") <(echo "$fields"))
  if [[ -n "$missing" ]]; then
    log "[ERROR] Schema drift detected, missing fields: $missing"
    send_alert "BOXOFFICE monitor failed: schema drift, missing fields: $missing"
  else
    log "[OK] Schema matches expected fields"
  fi
  rm -f "$tmp"
}

main() {
  load_env
  run_tests
  probe_upstream
  probe_schema
}

main "$@"
