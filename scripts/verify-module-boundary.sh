#!/usr/bin/env bash
# 机械校验跨服务 import 边界：
#   - contracts 不得依赖任何 services/*
#   - platform 不得依赖 contracts，也不得依赖任何业务服务
#   - 业务服务之间不得互相依赖（只允许依赖 contracts/platform）
# 用法：GO_BIN=/path/to/go bash scripts/verify-module-boundary.sh
set -euo pipefail
cd "$(dirname "$0")/.."

GO_BIN="${GO_BIN:-go}"
if ! "$GO_BIN" version >/dev/null 2>&1; then
  echo "FAIL: go 不可用（设置 GO_BIN 指向 go-dist 的 go）"
  exit 1
fi

BUSINESS_SERVICES=(console-bff service-registry monitor-svc episode-svc task-orchestrator)
ALL_SERVICES=(contracts platform "${BUSINESS_SERVICES[@]}")

fail=0
violations=0

deps_of() {
  "$GO_BIN" list -deps "./go/services/$1/..." 2>/dev/null || true
}

check_forbidden() {
  local svc="$1" pattern="$2" reason="$3"
  local count
  count=$(deps_of "$svc" | grep -Ec "$pattern" || true)
  if [ "$count" -ne 0 ]; then
    echo "VIOLATION: go/services/$svc 依赖 $reason（$count 处）"
    violations=$((violations + count))
    fail=1
  fi
}

# contracts 必须与所有服务解耦。
for other in "${ALL_SERVICES[@]}"; do
  [ "$other" = "contracts" ] && continue
  check_forbidden contracts "services/$other(\$|/)" "services/$other"
done

# platform 必须与 contracts 及所有业务服务解耦。
check_forbidden platform "services/contracts(\$|/)" "services/contracts"
for other in "${BUSINESS_SERVICES[@]}"; do
  check_forbidden platform "services/$other(\$|/)" "services/$other"
done

# 业务服务之间禁止互相依赖。
for svc in "${BUSINESS_SERVICES[@]}"; do
  for other in "${BUSINESS_SERVICES[@]}"; do
    [ "$svc" = "$other" ] && continue
    check_forbidden "$svc" "services/$other(\$|/)" "services/$other"
  done
done

if [ "$fail" -ne 0 ]; then
  echo "module-boundary: FAIL（$violations 处违规）"
  exit 1
fi

echo "module-boundary: PASS（contracts/platform/业务服务 均无禁止依赖）"
exit 0