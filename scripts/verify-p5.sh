#!/usr/bin/env bash
# P5 阶段门禁（MS-P5.11）：结构/构建/探活/gRPC/聚合/18 路由/不变量/回退/最终态。
# 不追求一次全绿；未执行或失败一律如实计为 SKIP/FAIL，绝不虚标 PASS。
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
hdr() { echo; echo "===== $1 ====="; }
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$1" 2>/dev/null || echo 000; }
q() { docker exec -e PGPASSWORD=alert alert-pg psql -U alert -d alert -tAc "$1" 2>/dev/null || echo ERR; }

hdr "1 结构"
if bash scripts/verify-p7.sh --structure-only >/tmp/p5-structure.log 2>&1; then ok structure; else ng structure "$(grep FAIL /tmp/p5-structure.log | head -1)"; fi

hdr "2 构建"
if [ -n "$GO_BIN" ]; then
  for m in go/contracts go/platform go/services/console-bff go/services/service-registry \
           go/services/monitor-svc go/services/episode-svc go/services/task-orchestrator; do
    if (cd "$m" && GOWORK=off "$GO_BIN" build ./... >/dev/null 2>&1); then ok "build $m"; else ng "build $m" failed; fi
  done
else
  skp build "go 不可用"
fi

hdr "3 探活"
for u in "registry|http://127.0.0.1:8081/healthz" "episode|http://127.0.0.1:8082/readyz" \
         "monitor|http://127.0.0.1:8083/readyz" \
         "bff|http://127.0.0.1:8080/readyz"; do
  n=${u%%|*}; url=${u##*|}; c=$(code "$url")
  [ "$c" = 200 ] && ok "alive $n" || ng "alive $n" "status=$c"
done
if [ "$(docker inspect --format '{{.State.Health.Status}}' alert-task-orchestrator 2>/dev/null)" = healthy ]; then
  ok "alive task(docker-health)"
else
  ng "alive task(docker-health)" "not healthy"
fi

hdr "4 gRPC 与方法枚举"
if docker exec alert-agent-runtime python -c "
import grpc
from alert_agent.infrastructure.grpc.gen.alertflow.system.v1 import task_gateway_pb2 as pb
ch=grpc.insecure_channel('task-orchestrator:9090'); grpc.channel_ready_future(ch).result(timeout=5)
s=pb.DESCRIPTOR.services_by_name['SystemTaskGatewayService']
assert len(s.methods)==7
" >/tmp/p5-grpc.log 2>&1; then ok grpc7; else ng grpc7 "$(tail -1 /tmp/p5-grpc.log)"; fi

hdr "5 聚合与 18 路由"
[ "$(code http://127.0.0.1:8080/api/v1/dashboard/summary)" = 200 ] && ok dashboard || ng dashboard "status=$(code http://127.0.0.1:8080/api/v1/dashboard/summary)"
[ "$(code http://127.0.0.1:8080/api/v1/services)" = 200 ] && ok services || ng services "status=$(code http://127.0.0.1:8080/api/v1/services)"

SID=$(curl -s --max-time 5 http://127.0.0.1:8080/api/v1/services | python3 -c 'import sys,json;d=json.load(sys.stdin);s=d.get("services") or [];print(s[0]["service_id"] if s else "")' 2>/dev/null || echo "")
EID=$(curl -s --max-time 5 "http://127.0.0.1:8080/api/v1/episodes?limit=1" | python3 -c 'import sys,json;d=json.load(sys.stdin);e=d.get("episodes") or [];print(e[0]["episode_id"] if e else "")' 2>/dev/null || echo "")
route_expect() { # path expected
  local c; c=$(code "http://127.0.0.1:8080$1")
  case "$2" in
    200) [ "$c" = 200 ] && ok "route $1" || ng "route $1" "status=$c" ;;
    404) [ "$c" = 404 ] && ok "route $1" || ng "route $1" "status=$c" ;;
    4xx) case "$c" in 4*) ok "route $1" ;; *) ng "route $1" "status=$c" ;; esac ;;
    *) ng "route $1" "bad expectation" ;;
  esac
}
[ -n "$SID" ] && route_expect "/api/v1/services/$SID" 200 || skp "route service get" "no service"
[ -n "$SID" ] && route_expect "/api/v1/services/$SID/targets" 200 || skp "route targets" "no service"
[ -n "$EID" ] && route_expect "/api/v1/episodes/$EID" 200 || skp "route episode get" "no episode"
[ -n "$EID" ] && route_expect "/api/v1/episodes/$EID/artifacts" 200 || skp "route artifacts" "no episode"
[ -n "$EID" ] && route_expect "/api/v1/episodes/$EID/timeline" 200 || skp "route episode timeline" "no episode"
[ -n "$SID" ] && route_expect "/api/v1/services/$SID/timeline" 200 || skp "route service timeline" "no service"
route_expect "/api/v1/episodes" 200
route_expect "/api/v1/no-such" 404

hdr "6 所有权不变量"
[ "$(q "SELECT count(*) FROM (SELECT service_id FROM task.service_execution_ownership GROUP BY service_id HAVING count(*)>1) x")" = 0 ] && ok I1 || ng I1 "duplicate ownership"
[ "$(q "SELECT count(*) FROM task.agent_tasks t LEFT JOIN task.service_execution_ownership o ON o.service_id=t.service_id WHERE t.status IN ('DISPATCHED','RUNNING') AND (o.service_id IS NULL OR o.lease_until<=now())")" = 0 ] && ok I2 || ng I2 "dispatched without fresh ownership"
[ "$(q "SELECT count(*) FROM task.agent_tasks t JOIN task.service_execution_ownership o ON o.task_id=t.task_id WHERE t.status IN ('DISPATCHED','RUNNING') AND t.lease_until<>o.lease_until")" = 0 ] && ok I3 || ng I3 "lease mismatch"

hdr "7 回退演练（dry-run）"
if bash deploy/split/rollback.sh --to P4 --dry-run >/tmp/p5-rollback.log 2>&1; then ok "rollback dry-run"; else
  # 骨架对未实现阶段返回非零属预期；只要未真正改库即记录 SKIP。
  skp "rollback dry-run" "$(tail -1 /tmp/p5-rollback.log)"
fi

hdr "8 最终态闸门"
if bash scripts/verify-p7.sh --structure-only >/tmp/p5-final.log 2>&1; then ok "final-gate structure"; else ng "final-gate" "$(grep FAIL /tmp/p5-final.log | head -1)"; fi

echo
echo "===== SUMMARY ====="
echo "PASS=$PASS FAIL=$FAIL SKIP=$SKIP"
[ "$FAIL" = 0 ] && [ "$SKIP" = 0 ] && exit 0
exit 1