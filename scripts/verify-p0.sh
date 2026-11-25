#!/usr/bin/env bash
# P0 阶段门槛：串行验证并输出 PASS/FAIL/SKIP 三态；任一 FAIL 非零退出。
# 用法：GO_BIN=/path/to/go bash scripts/verify-p0.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
DATABASE_URL="${TEST_DATABASE_URL:-postgres://alert:alert@127.0.0.1:5432/alert_test?sslmode=disable}"

pass=0
fail=0
skip=0
first_fail=""

check() {
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

mark_skip() {
  echo "SKIP $1：$2"
  skip=$((skip + 1))
}

build_all() {
  "$GO_BIN" build -C "$ROOT" \
    ./services/contracts/... ./services/platform/... ./services/system-core/...
}

vet_all() {
  "$GO_BIN" vet -C "$ROOT" \
    ./services/contracts/... ./services/platform/... ./services/system-core/...
}

contracts_test() { "$GO_BIN" test -C "$ROOT/services/contracts" ./... -count=1; }
platform_test() { "$GO_BIN" test -C "$ROOT/services/platform" ./... -count=1; }
boundary_check() { GO_BIN="$GO_BIN" bash "$ROOT/scripts/verify-module-boundary.sh"; }

p0_monitor() { "$GO_BIN" test -C "$ROOT/services/system-core" ./internal/application/monitor/ -run TestP0 -count=1; }
p0_episode() { "$GO_BIN" test -C "$ROOT/services/system-core" ./internal/application/episode/ -run TestP0 -count=1; }
p0_task() { "$GO_BIN" test -C "$ROOT/services/system-core" ./internal/application/task/ -run TestP0 -count=1; }

provision_check() {
  docker compose -f "$ROOT/deploy/compose/compose.yaml" --profile setup run --rm provision
}
migrate_idempotent() {
  docker exec -e PGPASSWORD=alert alert-pg psql -U alert -c "DROP DATABASE IF EXISTS alert_migrate_check" \
    -c "CREATE DATABASE alert_migrate_check" >/dev/null || return 1
  local dsn="postgres://alert:alert@127.0.0.1:5432/alert_migrate_check?sslmode=disable"
  local first second
  first="$(DATABASE_URL="$dsn" KAFKA_BROKERS=127.0.0.1:9092 "$GO_BIN" -C "$ROOT" run ./services/system-core/cmd/api -migrate 2>/dev/null | tail -1)"
  second="$(DATABASE_URL="$dsn" KAFKA_BROKERS=127.0.0.1:9092 "$GO_BIN" -C "$ROOT" run ./services/system-core/cmd/api -migrate 2>/dev/null | tail -1)"
  echo "first=$first second=$second"
  [ "$first" = "applied=3 skipped=0 conflicts=0" ] && [ "$second" = "applied=0 skipped=3 conflicts=0" ]
}
snapshot_reproducible() {
  bash "$ROOT/scripts/snapshot-routes.sh" &&
    bash "$ROOT/scripts/snapshot-routes.sh" &&
    bash "$ROOT/scripts/snapshot-diff.sh"
}

echo "== P0 门槛验证 =="
check build build_all
check vet vet_all
check contracts contracts_test
check platform platform_test
check module-boundary boundary_check

if [ -n "$DATABASE_URL" ] && timeout 5 docker info >/dev/null 2>&1; then
  check p0-monitor p0_monitor
  check p0-episode p0_episode
  check p0-task p0_task
else
  # 预演测试本身不依赖容器；Redis/PG 未就绪时 go test 会自动 skip 真实 PG 用例。
  check p0-monitor p0_monitor
  check p0-episode p0_episode
  check p0-task p0_task
fi

if timeout 5 docker info >/dev/null 2>&1; then
  check provision provision_check
  check isolation bash "$ROOT/deploy/split/verify-isolation.sh"
  S0_OUT="$(bash "$ROOT/deploy/split/migrate-data.sh" --dry-run --stage P0 2>&1)" || true
  if printf '%s' "$S0_OUT" | grep -q "S0 预检完成"; then
    echo "PASS split-preflight"
    pass=$((pass + 1))
  else
    echo "FAIL split-preflight"
    fail=$((fail + 1))
    first_fail="${first_fail:-split-preflight}"
  fi
  check migration-idempotent migrate_idempotent
else
  mark_skip provision "Docker daemon 不可用"
  mark_skip isolation "Docker daemon 不可用"
  mark_skip split-preflight "Docker daemon 不可用"
  mark_skip migration-idempotent "Docker daemon 不可用"
fi

if curl -sS --max-time 2 -o /dev/null "$BASE_URL/healthz" 2>/dev/null; then
  check snapshot-reproducible snapshot_reproducible
else
  mark_skip snapshot-reproducible "system-core api 未在 $BASE_URL 运行"
fi

echo "== 汇总：PASS=$pass FAIL=$fail SKIP=$skip =="
if [ "$fail" -ne 0 ]; then
  echo "P0 门槛失败，首个失败项：$first_fail"
  exit 1
fi
echo "P0 门槛通过（SKIP 项需在依赖就绪后补跑）"
exit 0