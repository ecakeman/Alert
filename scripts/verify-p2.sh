#!/usr/bin/env bash
# P2 阶段门槛：registry 域端点全错误分支 + 账号隔离 + compose.p2 存档 + BFF 可复现。
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

registry_probe() { /usr/bin/python3 "$ROOT/scripts/p2-probe.py"; }
isolation_check() { bash "$ROOT/deploy/split/verify-isolation.sh"; }
compose_archive_check() { docker compose -f "$ROOT/deploy/compose/compose.p2.yaml" config -q; }

run registry-probe registry_probe
run isolation isolation_check
run compose-p2-archive compose_archive_check

echo "== P2 汇总：PASS=$pass FAIL=$fail =="
if [ "$fail" -ne 0 ]; then
  echo "P2 门槛失败，首个失败项：$first_fail"
  exit 1
fi
echo "P2 门槛通过"
exit 0