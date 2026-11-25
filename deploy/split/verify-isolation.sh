#!/usr/bin/env bash
# 账号级 schema 隔离验证：4 条反向（跨 schema 必被拒）+ 4 条正向（自有 schema 可读写）。
# 依赖：provision 已执行；Docker 中的 alert-pg 可访问。
set -uo pipefail

PG_CONTAINER="${PG_CONTAINER:-alert-pg}"
PGDATABASE="${PGDATABASE:-alert}"

REGISTRY_PASSWORD="${ALERT_REGISTRY_PASSWORD:-alert_registry_pw_dev}"
MONITOR_PASSWORD="${ALERT_MONITOR_PASSWORD:-alert_monitor_pw_dev}"
EPISODE_PASSWORD="${ALERT_EPISODE_PASSWORD:-alert_episode_pw_dev}"
TASK_PASSWORD="${ALERT_TASK_PASSWORD:-alert_task_pw_dev}"

pass=0
total=8

run_as() {
  local role="$1" password="$2" sql="$3"
  docker exec -e PGPASSWORD="$password" "$PG_CONTAINER" \
    psql --username="$role" --dbname="$PGDATABASE" --no-psqlrc --tuples-only --no-align \
    --command="$sql" 2>&1
}

expect_denied() {
  local desc="$1" role="$2" password="$3" sql="$4"
  local out status
  out=$(run_as "$role" "$password" "$sql")
  status=$?
  if [ "$status" -ne 0 ] && printf '%s' "$out" | grep -qi "permission denied"; then
    pass=$((pass + 1))
    echo "PASS 反向: $desc"
  else
    echo "FAIL 反向: $desc（exit=$status output=$out）"
  fi
}

expect_allowed() {
  local desc="$1" role="$2" password="$3" sql="$4"
  local out status
  out=$(run_as "$role" "$password" "$sql")
  status=$?
  if [ "$status" -eq 0 ]; then
    pass=$((pass + 1))
    echo "PASS 正向: $desc"
  else
    echo "FAIL 正向: $desc（exit=$status output=$out）"
  fi
}

# ---------- 反向：跨 schema 读/写必须被拒 ----------
expect_denied "registry 读 monitor" alert_registry "$REGISTRY_PASSWORD" \
  "SELECT 1 FROM monitor.monitor_points LIMIT 1"
expect_denied "monitor 写 registry" alert_monitor "$MONITOR_PASSWORD" \
  "CREATE TABLE registry.__forbidden (id int)"
expect_denied "episode 读 task" alert_episode "$EPISODE_PASSWORD" \
  "SELECT 1 FROM task.agent_tasks LIMIT 1"
expect_denied "task 写 episode" alert_task "$TASK_PASSWORD" \
  "CREATE TABLE episode.__forbidden (id int)"

# ---------- 正向：自有 schema 建表/写入/读取（含 DEFAULT PRIVILEGES 与序列） ----------
forward_domain() {
  local schema="$1" role="$2" password="$3"
  local setup_sql cleanup_sql probe_sql
  setup_sql="CREATE TABLE ${schema}.__verify_owned (id bigserial PRIMARY KEY, v text)"
  probe_sql="INSERT INTO ${schema}.__verify_owned(v) VALUES ('ok'); SELECT v FROM ${schema}.__verify_owned ORDER BY id DESC LIMIT 1"
  cleanup_sql="DROP TABLE IF EXISTS ${schema}.__verify_owned"

  if ! docker exec "$PG_CONTAINER" psql --username=alert --dbname="$PGDATABASE" --no-psqlrc \
      --set=ON_ERROR_STOP=1 --command="$setup_sql" >/dev/null 2>&1; then
    echo "FAIL 正向: $schema 初始化表失败（alert 建表）"
    return
  fi
  local out status
  out=$(run_as "$role" "$password" "$probe_sql")
  status=$?
  docker exec "$PG_CONTAINER" psql --username=alert --dbname="$PGDATABASE" --no-psqlrc \
    --command="$cleanup_sql" >/dev/null 2>&1 || true

  if [ "$status" -eq 0 ] && printf '%s' "$out" | grep -q "ok"; then
    pass=$((pass + 1))
    echo "PASS 正向: $schema 读写自有表（default privileges + sequence）"
  else
    echo "FAIL 正向: $schema 读写失败（exit=$status output=$out）"
  fi
}

forward_domain registry alert_registry "$REGISTRY_PASSWORD"
forward_domain monitor alert_monitor "$MONITOR_PASSWORD"
forward_domain episode alert_episode "$EPISODE_PASSWORD"
forward_domain task alert_task "$TASK_PASSWORD"

echo "isolation: PASS $pass/$total"
if [ "$pass" -ne "$total" ]; then
  exit 1
fi
exit 0