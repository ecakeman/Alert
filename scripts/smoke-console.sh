#!/usr/bin/env bash
# P1.6 控制台主路径冒烟（curl 断言）；SMOKE_DEGRADED=1 时只验证下游不可达返回 503。
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
MODE="${SMOKE_DEGRADED:-0}"

SVC_1="11111111-1111-1111-1111-111111111111"
EP_1="33333333-3333-3333-3333-333333333333"

pass=0
fail=0

request() {
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "$BASE_URL$path"
  else
    curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X "$method" "$BASE_URL$path"
  fi
}

expect_2xx() {
  local name="$1" method="$2" path="$3" body="${4:-}"
  local status
  status="$(request "$method" "$path" "$body")"
  if [ "$status" -ge 200 ] && [ "$status" -lt 300 ]; then
    echo "PASS $name ($status)"
    pass=$((pass + 1))
  else
    echo "FAIL $name ($status)"
    fail=$((fail + 1))
  fi
}

if [ "$MODE" = "1" ]; then
  status="$(request GET /api/v1/dashboard/summary)"
  if [ "$status" = "503" ]; then
    echo "PASS dashboard-degraded (503)"
    pass=$((pass + 1))
  else
    echo "FAIL dashboard-degraded ($status，期望 503)"
    fail=$((fail + 1))
  fi
  echo "smoke: PASS=$pass FAIL=$fail（degraded）"
  [ "$fail" -eq 0 ] || exit 1
  exit 0
fi

bash "$ROOT/scripts/seed-baseline.sh" --reset >/dev/null

expect_2xx "services-list" GET "/api/v1/services"
expect_2xx "service-detail" GET "/api/v1/services/$SVC_1"
expect_2xx "service-create" POST "/api/v1/services" \
  '{"service_key":"smoke-created","name":"Smoke Created","description":"smoke","authority_producer_id":"producer-smoke","targets":[]}'
expect_2xx "target-create" POST "/api/v1/services/$SVC_1/targets" \
  '{"target_key":"smoke-target","target_name":"Smoke Target","target_type":"HTTP","address":"http://127.0.0.1:18085/healthz","interval_ms":15000,"timeout_ms":3000,"failure_threshold":2,"recovery_threshold":2,"is_required":false,"enabled":true}'
expect_2xx "episodes-list" GET "/api/v1/episodes"
expect_2xx "episode-detail" GET "/api/v1/episodes/$EP_1"
expect_2xx "episode-timeline" GET "/api/v1/episodes/$EP_1/timeline"
expect_2xx "episode-artifacts" GET "/api/v1/episodes/$EP_1/artifacts"
expect_2xx "dashboard-summary" GET "/api/v1/dashboard/summary"

echo "smoke: PASS=$pass FAIL=$fail"
[ "$fail" -eq 0 ] || exit 1
exit 0