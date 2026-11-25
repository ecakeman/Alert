#!/usr/bin/env bash
# P1 阶段门槛：P0 全量 + BFF 路由回归 + 控制台冒烟 + compose.p1 存档校验。
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
pass=0
fail=0
first_fail=""

run() {
  local name="$1"
  shift
  echo ">>> $name"
  if "$@"; then
    echo "PASS $name"
    pass=$((pass + 1))
  else
    echo "FAIL $name"
    fail=$((fail + 1))
    if [ -z "$first_fail" ]; then
      first_fail="$name"
    fi
  fi
}

compose_archive_check() {
  docker compose -f "$ROOT/deploy/compose/compose.p1.yaml" config -q
}

run p0 bash "$ROOT/scripts/verify-p0.sh"
run p1-routes bash "$ROOT/scripts/verify-p1-routes.sh"
run console-smoke bash "$ROOT/scripts/smoke-console.sh"
run compose-p1-archive compose_archive_check

echo "== P1 汇总：PASS=$pass FAIL=$fail =="
if [ "$fail" -ne 0 ]; then
  echo "P1 门槛失败，首个失败项：$first_fail"
  exit 1
fi
echo "P1 门槛通过"
exit 0