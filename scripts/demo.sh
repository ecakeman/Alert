#!/usr/bin/env bash
# 一键演示：真实执行（非仅打印）。--minimal 走精简闭环（readyz + 列表 + 已存在 episode/task 观测）。
# 退出码：0 全流程达成 / 1 超时或失败 / 2 环境不满足。
set -uo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
SVC_KEY="${DEMO_SERVICE_KEY:-demo-svc}"
MINIMAL=false
[ "${1:-}" = "--minimal" ] && MINIMAL=true

say() { echo "[demo] $*"; }
fail() { echo "[demo][FAIL] $*" >&2; exit 1; }

code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$BASE_URL/healthz" 2>/dev/null || echo 000)
[ "$code" = "200" ] || { echo "[demo] BFF healthz=$code"; exit 2; }
say "readyz=200"

SVC_ID=""
if [ "$MINIMAL" = false ]; then
  body=$(curl -s -o /tmp/demo-create.json -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
    -d "{\"service_key\":\"$SVC_KEY\",\"name\":\"Demo Service\",\"description\":\"demo\"}" "$BASE_URL/api/v1/services")
  say "create service http=$body"
  [ "$body" = "201" ] || [ "$body" = "409" ] || fail "建服务失败 http=$body"
  SVC_ID=$(jq -r '.service_id // empty' /tmp/demo-create.json 2>/dev/null)
  if [ -z "$SVC_ID" ]; then
    SVC_ID=$(curl -s "$BASE_URL/api/v1/services?limit=200&offset=0" | jq -r --arg k "$SVC_KEY" '.services[]|select(.service_key==$k)|.service_id' | head -1)
  fi
  [ -n "$SVC_ID" ] || fail "未取得 demo 服务 id"
  tbody=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
    -d '{"target_key":"demo-http","target_name":"Demo HTTP","address":"http://127.0.0.1:1/healthz","target_type":"HTTP","interval_ms":15000,"timeout_ms":3000,"failure_threshold":1,"recovery_threshold":1,"is_required":true}' \
    "$BASE_URL/api/v1/services/$SVC_ID/targets")
  say "create target http=$tbody"; [ "$tbody" = "201" ] || [ "$tbody" = "409" ] || fail "建目标失败 http=$tbody"
else
  say "minimal：跳过建服务，使用现有数据"
fi

say "services list:"
curl -s "$BASE_URL/api/v1/services?limit=5&offset=0" | jq -c '{total, first:(.services[0]//null)}'

say "episodes list:"
curl -s "$BASE_URL/api/v1/episodes?limit=3&offset=0" | jq -c '{total, first:(.episodes[0]//null)}'

EP_ID=$(curl -s "$BASE_URL/api/v1/episodes?limit=1&offset=0" | jq -r '.episodes[0].episode_id // empty')
if [ -n "$EP_ID" ]; then
  say "episode detail $EP_ID:"
  curl -s "$BASE_URL/api/v1/episodes/$EP_ID" | jq -c '{episode_id,status,service_name,active_task_status}'
  say "episode timeline $EP_ID keys:"
  curl -s "$BASE_URL/api/v1/episodes/$EP_ID/timeline" | jq -r 'keys|join(",")'
  say "artifacts $EP_ID:"
  curl -s "$BASE_URL/api/v1/episodes/$EP_ID/artifacts" | jq -c '{count:(.artifacts|length)}'
fi

say "入口: $BASE_URL  （trace 见 BFF access log）"
say "OK"
exit 0