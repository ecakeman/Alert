#!/usr/bin/env bash
# P6 阶段门禁（MS-P6.1 起）：直通路由逐字节 diff、D-24 结构断言、入口唯一。
# 用法：bash scripts/verify-p6.sh [--only=direct|all]  （目前两者等价，均只跑 direct 段）
# 退出码：0 全 PASS / 1 有 FAIL / 2 环境不满足（无 curl 或 BFF 不健康）
set -uo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
OUT_DIR="docs/evidence/p6/routes61"
MODE="all"
for arg in "$@"; do
  case "$arg" in
    --only=direct) MODE="direct" ;;
    --only=all) MODE="all" ;;
    --only=bridge) MODE="bridge" ;;
  esac
done

PASS=0; FAIL=0
ok() { PASS=$((PASS+1)); echo "PASS $1"; }
ng() { FAIL=$((FAIL+1)); echo "FAIL $1 :: $2"; }
hdr() { echo; echo "===== $1 ====="; }

if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
  echo "verify-p6 未执行（缺少 curl/jq）"; exit 2
fi

code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$BASE_URL/healthz" 2>/dev/null || echo 000)
if [ "$code" != "200" ]; then
  echo "verify-p6 未执行（BFF healthz=$code）"; exit 2
fi

if [ "$MODE" = "bridge" ]; then
  hdr "bridge 旧通道已停用"
  K="docker exec alert-kafka /opt/kafka/bin"
  if $K/kafka-topics.sh --bootstrap-server localhost:9092 --list 2>/dev/null | grep -q '^alertflow.system.events.v1$'; then
    echo "topic present（可接受，仅历史）"
  else
    echo "topic absent（可接受）"
  fi
  lag=$($K/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --all-groups 2>/dev/null | awk '$2=="alertflow.system.events.v1" && $6>0' | wc -l)
  [ "$lag" = "0" ] && ok "bridge_lag_zero" || ng "bridge_lag_zero" "LAG>0 组数=$lag"
  hits=$(grep -rn "alertflow.system.events" go/ python/ deploy/compose/ contracts/ --exclude-dir=system-core --include='*.go' --include='*.yaml' --include='*.py' --include='*.json' 2>/dev/null | wc -l)
  [ "$hits" = "0" ] && ok "bridge_config_zero" || ng "bridge_config_zero" "命中=$hits"
  sc=$(docker compose -f deploy/compose/compose.yaml config --format json 2>/dev/null | jq '[.services[]|select(.image//""|test("system-core"))]|length')
  [ "$sc" = "0" ] && ok "compose_no_system_core" || ng "compose_no_system_core" "system-core=$sc"
  if test -s deploy/compose/compose.p5.yaml && docker compose -f deploy/compose/compose.p5.yaml config >/dev/null 2>&1; then
    ok "compose_p5_parse"
  else
    ng "compose_p5_parse" "p5 不可解析"
  fi
  echo
  echo "===== SUMMARY ====="
  echo "PASS=$PASS FAIL=$FAIL"
  [ "$FAIL" = 0 ] && exit 0 || exit 1
fi

hdr "1 直通路由快照"
if OUT_DIR="$OUT_DIR" bash scripts/snapshot-routes.sh >/tmp/p6-snap.log 2>&1; then
  ok "snapshot"
else
  ng "snapshot" "$(tail -1 /tmp/p6-snap.log)"; echo "SUMMARY PASS=$PASS FAIL=$FAIL"; exit 1
fi

hdr "2 7 条直通逐字节 diff"
for f in 03_service_create 05_service_patch 06_service_delete 07_targets_list 08_target_create 09_target_patch 10_target_delete; do
  if diff -q "docs/evidence/p0/routes/$f.json" "$OUT_DIR/$f.json" >/dev/null 2>&1; then
    ok "diff $f"
  else
    ng "diff $f" "与 P0 基线不一致"
  fi
done

hdr "3 D-24 结构断言"
if jq -e 'type=="object" and (keys==["targets"]) and (.targets|type=="array")' "$OUT_DIR/07_targets_list.json" >/dev/null 2>&1; then
  ok "07_targets_wrapped"
else
  ng "07_targets_wrapped" "顶层不是 {targets:[...]}"
fi
for f in 03_service_create 05_service_patch 07_targets_list 08_target_create 09_target_patch; do
  if jq -e '[.. | objects | select(has("config_generation"))] | length == 0' "$OUT_DIR/$f.json" >/dev/null 2>&1; then
    ok "no_config_generation $f"
  else
    ng "no_config_generation $f" "L1 泄漏 config_generation"
  fi
done
if jq -e 'has("target_count") and has("last_check_at") and has("active_episode_id")' "$OUT_DIR/05_service_patch.json" >/dev/null 2>&1; then
  ok "05_three_fields"
else
  ng "05_three_fields" "05 缺 target_count/last_check_at/active_episode_id"
fi
if jq -e '(has("target_count")|not) and (has("last_check_at")|not) and (has("active_episode_id")|not)' "$OUT_DIR/03_service_create.json" >/dev/null 2>&1; then
  ok "03_omits_three"
else
  ng "03_omits_three" "03 不应出现三键"
fi

hdr "4 入口唯一（静态）"
DEFAULT_PORTS=$(docker compose -f deploy/compose/compose.yaml config --format json 2>/dev/null \
  | jq '[.services|to_entries[]|select(.key|test("service-registry|monitor-svc"))|(.value.ports//[])]|flatten|length' 2>/dev/null)
if [ "${DEFAULT_PORTS:-x}" = "0" ]; then ok "entry_unique_default"; else ng "entry_unique_default" "默认端口=$DEFAULT_PORTS"; fi

echo
echo "===== SUMMARY ====="
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" = 0 ] && exit 0 || exit 1