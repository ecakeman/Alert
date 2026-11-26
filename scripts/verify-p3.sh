#!/usr/bin/env bash
# P3 阶段门槛：monitor 拆分后的构建、迁移幂等、三态流转、点位一致性、P95 与账号隔离。
# 依赖：docker（alert-pg / alert-kafka）、WSL go（GO_BIN）。
# 任一断言失败即非零退出；输出 PASS/FAIL/未执行 三态。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
GO_BIN="${GO_BIN:-go}"
BIN_DIR="${BIN_DIR:-/tmp/alert-p3-bin}"
PG_CONTAINER="${PG_CONTAINER:-alert-pg}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-alert-kafka}"
mkdir -p "$BIN_DIR"

PSQL() { docker exec -i -e PGPASSWORD=alert "$PG_CONTAINER" psql -U alert -d alert -Atc "$1"; }
fail() { echo "FAIL: $*"; exit 1; }
pass() { echo "PASS: $*"; }
skip() { echo "未执行: $*"; }

MON_PID=""; REG_PID=""; WRK_PID=""
cleanup() {
  [ -n "$MON_PID" ] && kill "$MON_PID" 2>/dev/null || true
  [ -n "$REG_PID" ] && kill "$REG_PID" 2>/dev/null || true
  [ -n "$WRK_PID" ] && kill "$WRK_PID" 2>/dev/null || true
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  for _ in $(seq 1 60); do
    if curl -sf "$url" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  return 1
}

echo "== 1. 构建 =="
"$GO_BIN" build ./go/services/monitor-svc/... ./go/services/system-core/... || fail "go build"
pass "go build"

echo "== 2. monitor 迁移幂等 =="
"$GO_BIN" build -o "$BIN_DIR/monitor-svc" ./go/services/monitor-svc/cmd/monitor-svc || fail "build monitor-svc"
MON_DSN="postgres://alert_monitor:alert_monitor_pw_dev@127.0.0.1:5432/alert?sslmode=disable"
M1="$(DATABASE_URL="$MON_DSN" "$BIN_DIR/monitor-svc" -migrate 2>&1 | tail -1)" || fail "migrate #1"
M2="$(DATABASE_URL="$MON_DSN" "$BIN_DIR/monitor-svc" -migrate 2>&1 | tail -1)" || fail "migrate #2"
echo "$M1"; echo "$M2"
echo "$M2" | grep -q "applied=0" || fail "migrate 非幂等: $M2"
pass "migrate idempotent"

echo "== 3. 账号隔离 =="
bash deploy/split/verify-isolation.sh || fail "verify-isolation"
pass "isolation 8/8"

echo "== 4. 启动 registry / monitor / worker =="
"$GO_BIN" build -o "$BIN_DIR/service-registry" ./go/services/service-registry/cmd/service-registry || fail "build registry"
"$GO_BIN" build -o "$BIN_DIR/system-worker" ./go/services/system-core/cmd/worker || fail "build worker"
REG_DSN="postgres://alert_registry:alert_registry_pw_dev@127.0.0.1:5432/alert?sslmode=disable"
SYS_DSN="postgres://alert:alert@127.0.0.1:5432/alert?sslmode=disable"
DATABASE_URL="$REG_DSN" KAFKA_BROKERS=127.0.0.1:9092 HTTP_ADDR=:8081 "$BIN_DIR/service-registry" >/tmp/p3-registry.log 2>&1 &
REG_PID=$!
DATABASE_URL="$MON_DSN" KAFKA_BROKERS=127.0.0.1:9092 PROBE_ENABLED=false HTTP_ADDR=:8083 "$BIN_DIR/monitor-svc" >/tmp/p3-monitor.log 2>&1 &
MON_PID=$!
wait_http http://127.0.0.1:8081/healthz || fail "registry 未就绪"
wait_http http://127.0.0.1:8083/healthz || fail "monitor 未就绪"
DATABASE_URL="$SYS_DSN" KAFKA_BROKERS=127.0.0.1:9092 REMOTE_REGISTRY=true REGISTRY_BASE_URL=http://127.0.0.1:8081 \
  REMOTE_MONITOR=true MONITOR_BASE_URL=http://127.0.0.1:8083 PROBE_ENABLED=false AGENT_GRPC_TARGET=127.0.0.1:9090 \
  "$BIN_DIR/system-worker" >/tmp/p3-worker.log 2>&1 &
WRK_PID=$!
sleep 3
pass "services up"

echo "== 5. 建专用服务并跑三态流转 =="
KEY="p3-verify-$(date +%s)"
SID="$(curl -s -X POST -H 'Content-Type: application/json' \
  -d "{\"service_key\":\"$KEY\",\"name\":\"p3-verify\",\"authority_producer_id\":\"p3\"}" \
  http://127.0.0.1:8081/internal/services | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("service_id") or d.get("service",{}).get("service_id",""))')" \
  || fail "create service"
[ -n "$SID" ] || fail "create service: 无 service_id"
echo "service_id=$SID"

send_event() {
  local id="$1" status="$2" occ="$3" tmp
  tmp="$(mktemp)"
  printf '{"event_id":"%s","event_type":"monitor.service.state.changed.v1","schema_version":1,"aggregate_type":"service","aggregate_id":"%s","occurred_at":"%s","payload":{"service_id":"%s","status":"%s","aggregation_mode":"ALL_REQUIRED","target_health_digest":"normal=0,abnormal=1,unknown=0"}}\n' \
    "$id" "$SID" "$occ" "$SID" "$status" > "$tmp"
  docker exec -i "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server kafka:9092 --topic alertflow.monitor.events.v1 < "$tmp" 2>/dev/null
  rm -f "$tmp"
}

send_event "$(python3 -c 'import uuid;print(uuid.uuid4())')" ABNORMAL "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
for _ in $(seq 1 20); do
  ACTIVE="$(PSQL "select count(*) from system.episodes where service_id='$SID'::uuid and status='ACTIVE'")"
  [ "$ACTIVE" = "1" ] && break
  sleep 0.5
done
[ "$ACTIVE" = "1" ] || fail "ABNORMAL 未立案（active=$ACTIVE）"
pass "异常→ACTIVE"

EPID="$(PSQL "select episode_id from system.episodes where service_id='$SID'::uuid and status='ACTIVE' limit 1")"
curl -sf -o /dev/null -X POST -H 'Content-Type: application/json' \
  -d '{"source_type":"APP","status":"NORMAL","reason":"p3-verify"}' \
  "http://127.0.0.1:8081/internal/services/$SID/source-states" || fail "APP 状态写入"
PSQL "update system.episodes set input_version=1, final_input_version=1, processed_input_version=1, updated_at=now() where episode_id='$EPID'::uuid" >/dev/null

send_event "$(python3 -c 'import uuid;print(uuid.uuid4())')" NORMAL "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
RESOLVED="pending"
for _ in $(seq 1 20); do
  RESOLVED="$(PSQL "select status from system.episodes where episode_id='$EPID'::uuid")"
  [ "$RESOLVED" = "RESOLVED" ] && break
  sleep 0.5
done
[ "$RESOLVED" = "RESOLVED" ] || fail "NORMAL 未解除（status=$RESOLVED）"
pass "恢复→RESOLVED"

echo "== 6. 点位一致性 =="
LATEST="$(curl -s "http://127.0.0.1:8083/internal/services/$SID/points/latest-seq" | python3 -c 'import sys,json; print(json.load(sys.stdin)["latest_seq"])')"
MAXSEQ="$(PSQL "select coalesce(max(point_seq),0) from monitor.service_monitor_points where service_id='$SID'::uuid")"
[ "$LATEST" = "$MAXSEQ" ] || fail "latest-seq 不一致: api=$LATEST db=$MAXSEQ"
pass "latest-seq == max(point_seq) ($LATEST)"

echo "== 7. P95（/internal/points/trend × 20） =="
TMP_TIMES="$(mktemp)"
for _ in $(seq 1 20); do
  curl -s -o /dev/null -w '%{time_total}\n' "http://127.0.0.1:8083/internal/points/trend?window=24h&buckets=24" >> "$TMP_TIMES"
done
P95="$(sort -n "$TMP_TIMES" | awk '{a[NR]=$1} END{print a[int(NR*0.95)]}')"
rm -f "$TMP_TIMES"
echo "p95=${P95}s"
awk -v p="$P95" 'BEGIN{exit !(p <= 5.0)}' || fail "P95 超 5s: $P95"
pass "P95 <= 5s"

echo "== 8. compose 存档 =="
[ -f deploy/compose/compose.p3.yaml ] || fail "缺少 compose.p3.yaml"
docker compose -f deploy/compose/compose.p3.yaml config -q || fail "compose.p3.yaml 非法"
pass "compose.p3.yaml"

echo "P3 门槛通过"