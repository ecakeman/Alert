#!/usr/bin/env bash
# 创建 4 个域 schema 与 4 个登录角色，并在数据库层硬隔离跨域访问。
# 幂等：可重复执行；--down 删除 schema 与角色（仅用于本地清理）。
set -euo pipefail

PGHOST="${PGHOST:-postgres}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-alert}"
PGDATABASE="${PGDATABASE:-alert}"

REGISTRY_PASSWORD="${ALERT_REGISTRY_PASSWORD:-alert_registry_pw_dev}"
MONITOR_PASSWORD="${ALERT_MONITOR_PASSWORD:-alert_monitor_pw_dev}"
EPISODE_PASSWORD="${ALERT_EPISODE_PASSWORD:-alert_episode_pw_dev}"
TASK_PASSWORD="${ALERT_TASK_PASSWORD:-alert_task_pw_dev}"

psql_exec() {
  psql --host="$PGHOST" --port="$PGPORT" --username="$PGUSER" --dbname="$PGDATABASE" \
    --set=ON_ERROR_STOP=1 --no-psqlrc "$@"
}

ensure_role() {
  local role="$1" password="$2"
  if [ "$(psql_exec --tuples-only --no-align --command="SELECT 1 FROM pg_roles WHERE rolname='${role}'")" = "1" ]; then
    psql_exec --command="ALTER ROLE ${role} LOGIN PASSWORD :'pw'" --set="pw=${password}"
  else
    psql_exec --command="CREATE ROLE ${role} LOGIN PASSWORD :'pw'" --set="pw=${password}"
  fi
}

grant_domain() {
  local schema="$1" role="$2"
  psql_exec <<SQL
CREATE SCHEMA IF NOT EXISTS ${schema};
REVOKE ALL ON SCHEMA ${schema} FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA ${schema} TO ${role};
GRANT ALL ON ALL TABLES IN SCHEMA ${schema} TO ${role};
GRANT ALL ON ALL SEQUENCES IN SCHEMA ${schema} TO ${role};
ALTER DEFAULT PRIVILEGES IN SCHEMA ${schema} GRANT ALL ON TABLES TO ${role};
ALTER DEFAULT PRIVILEGES IN SCHEMA ${schema} GRANT ALL ON SEQUENCES TO ${role};
SQL
}

drop_domain() {
  local schema="$1" role="$2"
  psql_exec --command="DROP SCHEMA IF EXISTS ${schema} CASCADE"
  psql_exec --command="DROP ROLE IF EXISTS ${role}"
}

if [ "${1:-}" = "--down" ]; then
  drop_domain task alert_task
  drop_domain episode alert_episode
  drop_domain monitor alert_monitor
  drop_domain registry alert_registry
  echo "provision: down 完成"
  exit 0
fi

# public schema 不再向 PUBLIC 开放；system / cognitive 不动。
psql_exec --command="REVOKE ALL ON SCHEMA public FROM PUBLIC"

ensure_role alert_registry "$REGISTRY_PASSWORD"
ensure_role alert_monitor "$MONITOR_PASSWORD"
ensure_role alert_episode "$EPISODE_PASSWORD"
ensure_role alert_task "$TASK_PASSWORD"

grant_domain registry alert_registry
grant_domain monitor alert_monitor
grant_domain episode alert_episode
grant_domain task alert_task

psql_exec --command="SELECT rolname FROM pg_roles WHERE rolname LIKE 'alert\\_%' ORDER BY 1"
echo "provision: 4 schema / 4 role 就绪"