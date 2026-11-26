#!/usr/bin/env bash
# P4 阶段门禁（MS-P4.10）：结构/构建/用例行为/派发快照/BFF 路由/编排。
# 依赖本机已启动：registry:8081、episode-svc:8082、monitor:8083、单体 api:8084、BFF:8080、PG/Kafka 容器。
set -uo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

GO_BIN=${GO_BIN:-}
if [ -z "$GO_BIN" ]; then
  for c in "$HOME/go-dist/go/bin/go" "$(command -v go 2>/dev/null || true)"; do
    if [ -n "$c" ] && [ -x "$c" ]; then GO_BIN="$c"; break; fi
  done
fi

PASS=0; FAIL=0; SKIP=0
ok()  { PASS=$((PASS+1)); echo "PASS $1"; }
ng()  { FAIL=$((FAIL+1)); echo "FAIL $1 :: $2"; }
skp() { SKIP=$((SKIP+1)); echo "SKIP $1 :: $2"; }

q() { docker exec -e PGPASSWORD=alert alert-pg psql -U alert -d alert -tAc "$1" 2>/dev/null; }
jget() { python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null || echo ""; }

hdr() { echo; echo "===== $1 ====="; }

# 1 结构门禁
hdr "1 结构（verify-p7 --structure-only）"
if bash scripts/verify-p7.sh --structure-only >/tmp/p4-structure.log 2>&1; then ok "structure"; else ng "structure" "$(grep FAIL /tmp/p4-structure.log | head -1)"; fi

# 2 构建矩阵（A/B）
hdr "2 构建（7 module × A/B）"
if [ -n "$GO_BIN" ]; then
  for m in go/contracts go/platform go/services/console-bff go/services/service-registry \
           go/services/monitor-svc go/services/episode-svc go/services/system-core; do
    if (cd "$m" && GOWORK=off "$GO_BIN" build ./... >/dev/null 2>&1); then ok "build $m"; else ng "build $m" "GOWORK=off build failed"; fi
  done
else
  skp "build" "go 不可用"
fi

# 3 服务探活
hdr "3 探活"
for u in "registry|http://127.0.0.1:8081/healthz" "episode|http://127.0.0.1:8082/readyz" \
         "monitor|http://127.0.0.1:8083/readyz" "api|http://127.0.0.1:8084/readyz" "bff|http://127.0.0.1:8080/readyz"; do
  name=${u%%|*}; url=${u##*|}
  code=$(curl -s -o /dev/null -w '%{http_code}' "$url")
  if [ "$code" = "200" ]; then ok "alive $name"; else ng "alive $name" "status=$code"; fi
done

# 4 用例行为
hdr "4 用例（立案/输入去重/快照/missing/路由）"
# 对账 worker 会主动为 pending 快照建任务，导致解除判定出现 ACTIVE_TASK_EXISTS（正确业务语义）。
# 本门禁测的是判定函数本身，先停 worker 消除时序竞争。
pkill -x alert-worker || true
sleep 1
TS=$(date +%s)
SID=$(curl -s -X POST http://127.0.0.1:8081/internal/services -H 'Content-Type: application/json' \
  -d "{\"service_key\":\"p4-gate-$TS\",\"name\":\"p4-gate\",\"authority_producer_id\":\"p4-gate\",\"targets\":[]}" | jget 'd["service_id"]')
if [ -z "$SID" ]; then
  ng "create service" "empty service_id"
else
  ok "create service"
  OPEN1=$(curl -s -X POST "http://127.0.0.1:8082/internal/services/$SID/episodes/open")
  EP=$(printf '%s' "$OPEN1" | jget 'd["episode"]["episode_id"]')
  OPEN2=$(curl -s -X POST "http://127.0.0.1:8082/internal/services/$SID/episodes/open")
  [ "$(printf '%s' "$OPEN1" | jget 'd["created"]')" = "True" ] && ok "open created" || ng "open created" "$OPEN1"
  [ "$(printf '%s' "$OPEN2" | jget 'd["created"]')" = "False" ] && ok "open reuse" || ng "open reuse" "$OPEN2"

  IV1=$(curl -s -X POST "http://127.0.0.1:8082/internal/episodes/$EP/input-versions")
  IV2=$(curl -s -X POST "http://127.0.0.1:8082/internal/episodes/$EP/input-versions")
  [ "$(printf '%s' "$IV1" | jget 'd["appended"]')" = "True" ] && ok "input append" || ng "input append" "$IV1"
  [ "$(printf '%s' "$IV2" | jget 'd["appended"]')" = "False" ] && ok "input dedupe" || ng "input dedupe" "$IV2"

  FIELDS=$(q "SELECT payload::jsonb->'artifact_readiness' IS NOT NULL AND payload::jsonb ? 'episode_id' AND payload::jsonb ? 'input_version_from' AND payload::jsonb ? 'input_version_to' AND payload::jsonb ? 'reason' FROM episode.outbox_events WHERE event_type='episode.dispatchable.v1' AND aggregate_id='$EP' ORDER BY created_at DESC LIMIT 1")
  [ "$FIELDS" = "t" ] && ok "dispatchable fields" || ng "dispatchable fields" "missing keys"

  for st in MONITOR APP; do
    curl -s -o /dev/null -X POST "http://127.0.0.1:8081/internal/services/$SID/source-states" \
      -H 'Content-Type: application/json' \
      -d "{\"service_id\":\"$SID\",\"source_type\":\"$st\",\"status\":\"NORMAL\",\"reason\":\"p4-gate\"}"
  done
  q "UPDATE episode.episodes SET input_version=1, final_input_version=1, processed_input_version=1, updated_at=now() WHERE episode_id='$EP'" >/dev/null
  RES=$(curl -s -X POST "http://127.0.0.1:8082/internal/episodes/$EP/resolve")
  [ "$(printf '%s' "$RES" | jget 'd["resolved"]')" = "True" ] && ok "resolve" || ng "resolve" "$RES"

  code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:8082/internal/episodes/pending-dispatch?limit=5")
  [ "$code" = "200" ] && ok "pending-dispatch" || ng "pending-dispatch" "status=$code"

  for u in "11|http://127.0.0.1:8080/api/v1/episodes?limit=1" \
           "12|http://127.0.0.1:8080/api/v1/episodes/$EP" \
           "13|http://127.0.0.1:8080/api/v1/services/$SID/events"; do
    n=${u%%|*}; url=${u##*|}
    if [ "$n" = "13" ]; then
      code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$url" -H 'Content-Type: application/json' \
        -d "{\"producer_id\":\"p4-gate\",\"event_id\":\"$(python3 -c 'import uuid;print(uuid.uuid4())')\",\"event_kind\":\"DEPLOY\",\"health_status\":\"NORMAL\",\"payload\":{}}")
    else
      code=$(curl -s -o /dev/null -w '%{http_code}' "$url")
    fi
    [ "$code" = "200" ] && ok "route $n" || ng "route $n" "status=$code"
  done
fi

# 5 编排
hdr "5 编排"
if docker compose -f deploy/compose/compose.p4.yaml config >/dev/null 2>&1; then ok "compose.p4"; else ng "compose.p4" "config failed"; fi
if grep -q 'episode-svc:' deploy/compose/compose.p4.yaml && ! grep -A20 'episode-svc:' deploy/compose/compose.p4.yaml | grep -q 'ports:'; then
  ok "episode no host port"
else
  ng "episode no host port" "ports mapping found"
fi

echo
echo "---- P4 门禁结束：PASS=$PASS FAIL=$FAIL SKIP=$SKIP ----"
[ "$FAIL" = "0" ] && exit 0 || exit 1