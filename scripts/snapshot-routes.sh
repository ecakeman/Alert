#!/usr/bin/env bash
# 采集 18 条成功路由 + 3 个错误分支的响应快照，JSON 键排序并归一化时间字段。
# 每次采集前重置固定 seed；上一次结果保留为 <out>.prev 供 snapshot-diff 比较。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
OUT_DIR="${OUT_DIR:-$ROOT/docs/evidence/p0/routes}"
DRY_RUN=false
SKIP_SEED=false
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --skip-seed) SKIP_SEED=true; shift ;;
    --base-url) BASE_URL="${2:?}"; shift 2 ;;
    --out) OUT_DIR="${2:?}"; shift 2 ;;
    *) echo "未知参数: $1"; exit 2 ;;
  esac
done

SVC_1="11111111-1111-1111-1111-111111111111"
SVC_2="11111111-1111-1111-1111-111111111112"
TGT_1="22222222-2222-2222-2222-222222222222"
TGT_2="22222222-2222-2222-2222-222222222223"
EP_1="33333333-3333-3333-3333-333333333333"
MISSING="99999999-9999-9999-9999-999999999999"

REQUESTS=(
  "01_dashboard_summary|GET|/api/v1/dashboard/summary|"
  "02_services_list|GET|/api/v1/services?limit=10&offset=0|"
  "03_service_create|POST|/api/v1/services|{\"service_key\":\"snapshot-created\",\"name\":\"Snapshot Created\",\"description\":\"created by snapshot\",\"authority_producer_id\":\"producer-snapshot\",\"targets\":[]}"
  "04_service_get|GET|/api/v1/services/$SVC_1|"
  "05_service_patch|PATCH|/api/v1/services/$SVC_1|{\"description\":\"baseline patched\"}"
  "06_service_delete|DELETE|/api/v1/services/$SVC_2|"
  "07_targets_list|GET|/api/v1/services/$SVC_1/targets|"
  "08_target_create|POST|/api/v1/services/$SVC_1/targets|{\"target_key\":\"snapshot-target\",\"target_name\":\"Snapshot Target\",\"target_type\":\"HTTP\",\"address\":\"http://127.0.0.1:18084/healthz\",\"interval_ms\":15000,\"timeout_ms\":3000,\"failure_threshold\":2,\"recovery_threshold\":2,\"is_required\":false,\"enabled\":true}"
  "09_target_patch|PATCH|/api/v1/targets/$TGT_1|{\"enabled\":false}"
  "10_target_delete|DELETE|/api/v1/targets/$TGT_2|"
  "11_episodes_list|GET|/api/v1/episodes?limit=10&offset=0|"
  "12_episode_get|GET|/api/v1/episodes/$EP_1|"
  "13_app_event|POST|/api/v1/services/$SVC_1/events|{\"producer_id\":\"producer-checkout\",\"event_id\":\"66666666-6666-6666-6666-666666666666\",\"event_kind\":\"DEPLOY\",\"health_status\":\"NORMAL\",\"payload\":{\"version\":\"v2\"},\"event_time\":\"2026-11-01T00:00:00Z\"}"
  "14_service_timeline|GET|/api/v1/services/$SVC_1/timeline?limit=10|"
  "15_episode_timeline|GET|/api/v1/episodes/$EP_1/timeline|"
  "16_artifacts_get|GET|/api/v1/episodes/$EP_1/artifacts|"
  "17_retry_diagnosis|POST|/api/v1/episodes/$EP_1/retry-diagnosis|"
  "18_retry_archive|POST|/api/v1/episodes/$EP_1/retry-archive|"
  "19_err_not_found|GET|/api/v1/services/$MISSING|"
  "20_err_conflict|POST|/api/v1/services|{\"service_key\":\"checkout-api\",\"name\":\"Duplicate\",\"description\":\"dup\",\"authority_producer_id\":\"producer-dup\",\"targets\":[]}"
  "21_err_validation|POST|/api/v1/services|{}"
)

if [ "$DRY_RUN" = true ]; then
  echo "snapshot: dry-run（$BASE_URL → $OUT_DIR）"
  for item in "${REQUESTS[@]}"; do
    IFS='|' read -r name method path body <<< "$item"
    echo "  $name $method $path"
  done
  exit 0
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "snapshot 未执行（环境：缺少 curl）"
  exit 2
fi

if [ "$SKIP_SEED" = false ]; then
  bash "$SCRIPT_DIR/seed-baseline.sh" --reset
fi

health_status="$(curl -sS --max-time 3 -o /dev/null -w '%{http_code}' "$BASE_URL/healthz" || true)"
if [ "$health_status" != "200" ]; then
  echo "snapshot 未执行（环境：$BASE_URL/healthz 返回 ${health_status:-连接失败}；请先启动 system-core api 并迁移）"
  exit 2
fi

# 归一化用 WSL python3；只从 stdin 读文件并写 stdout，文件写入交给 bash 重定向。
wsl_path() { printf '%s' "$1" | sed 's|^//wsl.localhost/[^/]*||'; }
if command -v python3 >/dev/null 2>&1; then
  PYTHON_CMD=(python3)
  PYTHON_TRANSLATE=false
else
  WSL_DISTRO="${WSL_DISTRO:-Ubuntu-24.04}"
  PYTHON_CMD=(wsl.exe -d "$WSL_DISTRO" -- /usr/bin/python3)
  PYTHON_TRANSLATE=true
fi

NORMALIZE_CODE='import json,re,sys
ts_re=re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}")
uuid_re=re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
hex32_re=re.compile(r"^[0-9a-f]{32}$")
seen={}
def norm(v):
    if isinstance(v,dict): return {k:norm(x) for k,x in v.items()}
    if isinstance(v,list): return [norm(x) for x in v]
    if isinstance(v,str):
        if ts_re.match(v): return "<timestamp>"
        if hex32_re.match(v): return "<hex32>"
        if uuid_re.match(v):
            if v not in seen: seen[v]="<uuid-%d>"%(len(seen)+1)
            return seen[v]
    return v
json.dump(norm(json.load(open(sys.argv[1],encoding="utf-8"))),sys.stdout,sort_keys=True,indent=2,ensure_ascii=False)
print()'

mkdir -p "$OUT_DIR"
if [ -d "$OUT_DIR" ]; then
  rm -rf "$OUT_DIR.prev"
  cp -r "$OUT_DIR" "$OUT_DIR.prev"
fi
rm -f "$OUT_DIR"/*.json "$OUT_DIR"/*.status 2>/dev/null || true

RAW_DIR="$(mktemp -d)"
trap 'rm -rf "$RAW_DIR"' EXIT

capture() {
  local name="$1" method="$2" path="$3" body="${4:-}"
  local raw="$RAW_DIR/$name.json" out="$OUT_DIR/$name.json" status
  if [ -n "$body" ]; then
    status=$(curl -sS --max-time 15 -o "$raw" -w '%{http_code}' -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "$BASE_URL$path")
  else
    status=$(curl -sS --max-time 15 -o "$raw" -w '%{http_code}' -X "$method" "$BASE_URL$path")
  fi
  printf '%s\n' "$status" > "$OUT_DIR/$name.status"
  if [ -s "$raw" ]; then
    local in_path="$raw"
    if [ "$PYTHON_TRANSLATE" = true ]; then in_path="$(wsl_path "$raw")"; fi
    "${PYTHON_CMD[@]}" -c "$NORMALIZE_CODE" "$in_path" > "$out"
  else
    : > "$out"
  fi
}

for item in "${REQUESTS[@]}"; do
  IFS='|' read -r name method path body <<< "$item"
  capture "$name" "$method" "$path" "$body"
done

echo "snapshot: 完成，输出到 $OUT_DIR（21 个响应）"