#!/usr/bin/env bash
# 存量搬迁：P0=S0 只读预检；P2=把 registry 三表从 system 迁入 registry（单事务）。
set -euo pipefail

SPLIT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SPLIT_DIR/../.." && pwd)"
PG_CONTAINER="${PG_CONTAINER:-alert-pg}"
PGDATABASE="${PGDATABASE:-alert}"
STATE_FILE="$SPLIT_DIR/.split-state.json"

DRY_RUN=false
STAGE="P0"
CONFIRM=false
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --stage) STAGE="${2:?--stage 需要值}"; shift 2 ;;
    --confirm) CONFIRM=true; shift ;;
    *) echo "未知参数: $1"; exit 2 ;;
  esac
done

ensure_db() {
  if ! docker exec -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
      --username=alert --dbname="$PGDATABASE" --no-psqlrc --tuples-only --no-align \
      --command="SELECT 1" >/dev/null 2>&1; then
    echo "未执行（环境：Docker daemon 或 $PG_CONTAINER 不可用）"
    exit 1
  fi
}

query() {
  docker exec -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
    --username=alert --dbname="$PGDATABASE" --no-psqlrc --tuples-only --no-align \
    --command="$1"
}

psql_exec() {
  docker exec -i -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
    --username=alert --dbname="$PGDATABASE" --no-psqlrc --set=ON_ERROR_STOP=1 "$@"
}

TABLES=(
  services
  service_monitor_targets
  service_source_states
  episodes
  episode_input_versions
  service_monitor_points
  episode_app_events
  agent_tasks
  service_execution_ownership
  outbox_events
  consumer_idempotency
  episode_artifact_refs
  episode_fault_document_refs
)

snapshot() {
  local stage="$1"
  local migration_version unpublished recent generated_at snapshot_file
  migration_version="$(query "SELECT coalesce(max(version),0) FROM system.schema_migrations")"
  unpublished="$(query "SELECT count(*) FROM system.outbox_events WHERE published_at IS NULL")"
  recent="$(query "SELECT count(*) FROM system.outbox_events WHERE created_at > now() - interval '5 minutes'")"

  mkdir -p "$SPLIT_DIR/snapshots"
  generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  snapshot_file="$SPLIT_DIR/snapshots/$(date -u +%Y%m%dT%H%M%SZ).json"

  {
    printf '{\n'
    printf '  "stage": "%s",\n' "$stage"
    printf '  "generated_at": "%s",\n' "$generated_at"
    printf '  "migration_version": %s,\n' "$migration_version"
    printf '  "active_writes": {"unpublished_outbox": %s, "recent_5m_outbox": %s},\n' \
      "$unpublished" "$recent"
    printf '  "tables": {\n'
    local last=$(( ${#TABLES[@]} - 1 )) i table rows size
    for i in "${!TABLES[@]}"; do
      table="${TABLES[$i]}"
      rows="$(query "SELECT count(*) FROM system.${table}")"
      size="$(query "SELECT pg_total_relation_size('system.${table}')")"
      if [ "$i" -eq "$last" ]; then
        printf '    "system.%s": {"rows": %s, "size_bytes": %s}\n' "$table" "$rows" "$size"
      else
        printf '    "system.%s": {"rows": %s, "size_bytes": %s},\n' "$table" "$rows" "$size"
      fi
    done
    printf '  }\n'
    printf '}\n'
  } > "$snapshot_file"
  printf '%s' "$snapshot_file"
}

write_state() {
  local stage="$1" phase="$2" snapshot="$3" tmp
  tmp="$(mktemp "$SPLIT_DIR/.split-state.json.XXXXXX")"
  printf '{"stage":"%s","phase":"%s","started_at":"%s","snapshot":"%s"}\n' \
    "$stage" "$phase" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${snapshot:-}" > "$tmp"
  mv "$tmp" "$STATE_FILE"
}

if [ "$STAGE" = "P2" ]; then
  if [ -f "$STATE_FILE" ] && grep -q '"stage":"P2"' "$STATE_FILE" && grep -q '"phase":"complete"' "$STATE_FILE"; then
    echo "P2 搬迁已完成（读状态文件，no-op）"
    exit 0
  fi
  ensure_db

  SNAPSHOT_FILE="$(snapshot P2)"
  echo "S0 预检完成：$SNAPSHOT_FILE"

  if [ "$(query "SELECT to_regclass('registry.services') IS NOT NULL")" = "t" ]; then
    rows="$(query "SELECT count(*) FROM registry.services")"
    if [ "$rows" != "0" ]; then
      echo "registry.services 已含 $rows 行，拒绝覆盖"
      exit 5
    fi
  fi

  before_services="$(query "SELECT count(*) FROM system.services")"
  before_targets="$(query "SELECT count(*) FROM system.service_monitor_targets")"
  before_sources="$(query "SELECT count(*) FROM system.service_source_states")"

  MIGRATE_SQL="$(cat <<'SQL'
ALTER TABLE system.service_monitor_points DROP CONSTRAINT IF EXISTS service_monitor_points_service_id_fkey;
ALTER TABLE system.service_monitor_points DROP CONSTRAINT IF EXISTS service_monitor_points_target_id_fkey;
ALTER TABLE system.episodes DROP CONSTRAINT IF EXISTS episodes_service_id_fkey;
ALTER TABLE system.episode_app_events DROP CONSTRAINT IF EXISTS episode_app_events_service_id_fkey;
ALTER TABLE system.agent_tasks DROP CONSTRAINT IF EXISTS agent_tasks_service_id_fkey;
ALTER TABLE system.agent_tasks DROP CONSTRAINT IF EXISTS agent_tasks_episode_id_fkey;
ALTER TABLE system.service_execution_ownership DROP CONSTRAINT IF EXISTS service_execution_ownership_service_id_fkey;
ALTER TABLE system.episode_artifact_refs DROP CONSTRAINT IF EXISTS episode_artifact_refs_episode_id_fkey;
ALTER TABLE system.episode_fault_document_refs DROP CONSTRAINT IF EXISTS episode_fault_document_refs_episode_id_fkey;
ALTER TABLE system.service_monitor_targets DROP CONSTRAINT IF EXISTS service_monitor_targets_service_id_fkey;
ALTER TABLE system.service_source_states DROP CONSTRAINT IF EXISTS service_source_states_service_id_fkey;

DROP TABLE IF EXISTS registry.service_monitor_targets CASCADE;
DROP TABLE IF EXISTS registry.service_source_states CASCADE;
DROP TABLE IF EXISTS registry.services CASCADE;

ALTER TABLE system.services SET SCHEMA registry;
ALTER TABLE system.service_monitor_targets SET SCHEMA registry;
ALTER TABLE system.service_source_states SET SCHEMA registry;

ALTER TABLE registry.service_monitor_targets
  ADD CONSTRAINT service_monitor_targets_service_id_fkey
  FOREIGN KEY (service_id) REFERENCES registry.services(service_id) ON DELETE RESTRICT;
ALTER TABLE registry.service_source_states
  ADD CONSTRAINT service_source_states_service_id_fkey
  FOREIGN KEY (service_id) REFERENCES registry.services(service_id) ON DELETE RESTRICT;
SQL
)"

  if [ "$DRY_RUN" = true ]; then
    printf '%s\n' "$MIGRATE_SQL"
    echo "[dry-run] P2 搬迁 SQL 打印完毕（未执行）"
    exit 0
  fi
  if [ "$CONFIRM" != true ]; then
    echo "拒绝执行危险动作（缺少 --confirm）"
    exit 4
  fi

  printf '%s\n' "$MIGRATE_SQL" | psql_exec --single-transaction >/dev/null

  psql_exec >/dev/null <<'SQL'
GRANT ALL ON registry.services TO alert_registry;
GRANT ALL ON registry.service_monitor_targets TO alert_registry;
GRANT ALL ON registry.service_source_states TO alert_registry;
SQL

  CHECKSUM="$(sha256sum "$ROOT/services/service-registry/migrations/0001_registry_core.sql" | awk '{print $1}')"
  psql_exec >/dev/null <<SQL
CREATE TABLE IF NOT EXISTS registry.schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    checksum text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now(),
    execution_ms bigint NOT NULL DEFAULT 0
);
INSERT INTO registry.schema_migrations(version,name,checksum) VALUES(1,'registry_core','${CHECKSUM}')
ON CONFLICT (version) DO NOTHING;
SQL

  after_services="$(query "SELECT count(*) FROM registry.services")"
  after_targets="$(query "SELECT count(*) FROM registry.service_monitor_targets")"
  after_sources="$(query "SELECT count(*) FROM registry.service_source_states")"
  if [ "$before_services" != "$after_services" ] || [ "$before_targets" != "$after_targets" ] || [ "$before_sources" != "$after_sources" ]; then
    echo "行数对账失败: services $before_services->$after_services, targets $before_targets->$after_targets, sources $before_sources->$after_sources"
    exit 6
  fi
  if [ "$(query "SELECT to_regclass('system.services') IS NOT NULL")" = "t" ]; then
    echo "system.services 仍存在，搬迁未完成"
    exit 7
  fi

  write_state P2 complete "$(basename "$SNAPSHOT_FILE")"
  echo "P2 搬迁完成：services=$after_services targets=$after_targets sources=$after_sources"
  echo "状态文件：$STATE_FILE（stage=P2 phase=complete）"
  exit 0
fi

# 默认：P0 = S0 只读预检
ensure_db
SNAPSHOT_FILE="$(snapshot P0)"
phase="s0-complete"
if [ "$DRY_RUN" = true ]; then
  phase="dry-run"
fi
write_state P0 "$phase" "$(basename "$SNAPSHOT_FILE")"
echo "S0 预检完成：$SNAPSHOT_FILE"
echo "状态文件：$STATE_FILE（phase=$phase）"