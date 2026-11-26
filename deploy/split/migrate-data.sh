#!/usr/bin/env bash
# 存量搬迁：P0=S0 只读预检；P2=registry 三表、P3=points、P4=episode 三表从 system 迁入本域（单事务）。
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
      # 前序阶段已搬走的表不再出现在 system，快照记 0 而非报错。
      if [ "$(query "SELECT to_regclass('system.${table}') IS NOT NULL")" = "t" ]; then
        rows="$(query "SELECT count(*) FROM system.${table}")"
        size="$(query "SELECT pg_total_relation_size('system.${table}')")"
      else
        rows=0
        size=0
      fi
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

if [ "$STAGE" = "P5" ]; then
  if [ -f "$STATE_FILE" ] && grep -q '"stage":"P5"' "$STATE_FILE" && grep -q '"phase":"complete"' "$STATE_FILE"; then
    echo "P5 搬迁已完成（读状态文件，no-op）"
    exit 0
  fi
  ensure_db

  for t in agent_tasks service_execution_ownership episode_artifact_refs episode_fault_document_refs; do
    if [ "$(query "SELECT to_regclass('task.${t}') IS NOT NULL")" != "t" ]; then
      echo "task.${t} 不存在：请先执行 task-orchestrator -migrate（P5.1）"
      exit 5
    fi
    rows="$(query "SELECT count(*) FROM task.${t}")"
    if [ "$rows" != "0" ]; then
      echo "task.${t} 已含 $rows 行，拒绝覆盖"
      exit 5
    fi
  done

  SNAPSHOT_FILE="$(snapshot P5)"
  echo "S0 预检完成：$SNAPSHOT_FILE"

  before_tasks="$(query "SELECT count(*) FROM system.agent_tasks")"
  before_ownership="$(query "SELECT count(*) FROM system.service_execution_ownership")"
  before_artifacts="$(query "SELECT count(*) FROM system.episode_artifact_refs")"
  before_docs="$(query "SELECT count(*) FROM system.episode_fault_document_refs")"

  MIGRATE_SQL="$(cat <<'SQL'
-- 同域 FK 先删（SET SCHEMA 要求引用表同 schema）
ALTER TABLE system.service_execution_ownership DROP CONSTRAINT IF EXISTS service_execution_ownership_task_id_fkey;
ALTER TABLE system.episode_artifact_refs DROP CONSTRAINT IF EXISTS episode_artifact_refs_task_id_fkey;
ALTER TABLE system.episode_fault_document_refs DROP CONSTRAINT IF EXISTS episode_fault_document_refs_task_id_fkey;

-- 目标域为已迁移的空表；删除以让 SET SCHEMA 接管（索引随表迁移）
DROP TABLE IF EXISTS task.episode_fault_document_refs CASCADE;
DROP TABLE IF EXISTS task.episode_artifact_refs CASCADE;
DROP TABLE IF EXISTS task.service_execution_ownership CASCADE;
DROP TABLE IF EXISTS task.agent_tasks CASCADE;

ALTER TABLE system.agent_tasks SET SCHEMA task;
ALTER TABLE system.service_execution_ownership SET SCHEMA task;
ALTER TABLE system.episode_artifact_refs SET SCHEMA task;
ALTER TABLE system.episode_fault_document_refs SET SCHEMA task;

ALTER TABLE task.service_execution_ownership
  ADD CONSTRAINT service_execution_ownership_task_id_fkey
  FOREIGN KEY (task_id) REFERENCES task.agent_tasks(task_id) ON DELETE RESTRICT;
ALTER TABLE task.episode_artifact_refs
  ADD CONSTRAINT episode_artifact_refs_task_id_fkey
  FOREIGN KEY (task_id) REFERENCES task.agent_tasks(task_id) ON DELETE RESTRICT;
ALTER TABLE task.episode_fault_document_refs
  ADD CONSTRAINT episode_fault_document_refs_task_id_fkey
  FOREIGN KEY (task_id) REFERENCES task.agent_tasks(task_id) ON DELETE RESTRICT;
SQL
)"

  if [ "$DRY_RUN" = true ]; then
    printf '%s\n' "$MIGRATE_SQL"
    echo "[dry-run] P5 搬迁 SQL 打印完毕（未执行）"
    exit 0
  fi
  if [ "$CONFIRM" != true ]; then
    echo "拒绝执行危险动作（缺少 --confirm）"
    exit 4
  fi

  printf '%s\n' "$MIGRATE_SQL" | psql_exec --single-transaction >/dev/null

  psql_exec >/dev/null <<'SQL'
GRANT ALL ON task.agent_tasks TO alert_task;
GRANT ALL ON task.service_execution_ownership TO alert_task;
GRANT ALL ON task.episode_artifact_refs TO alert_task;
GRANT ALL ON task.episode_fault_document_refs TO alert_task;
SQL

  after_tasks="$(query "SELECT count(*) FROM task.agent_tasks")"
  after_ownership="$(query "SELECT count(*) FROM task.service_execution_ownership")"
  after_artifacts="$(query "SELECT count(*) FROM task.episode_artifact_refs")"
  after_docs="$(query "SELECT count(*) FROM task.episode_fault_document_refs")"
  if [ "$before_tasks" != "$after_tasks" ] || [ "$before_ownership" != "$after_ownership" ] \
     || [ "$before_artifacts" != "$after_artifacts" ] || [ "$before_docs" != "$after_docs" ]; then
    echo "行数对账失败: tasks $before_tasks->$after_tasks, ownership $before_ownership->$after_ownership, artifacts $before_artifacts->$after_artifacts, docs $before_docs->$after_docs"
    exit 6
  fi
  for t in agent_tasks service_execution_ownership episode_artifact_refs episode_fault_document_refs; do
    if [ "$(query "SELECT to_regclass('system.${t}') IS NOT NULL")" = "t" ]; then
      echo "system.${t} 仍存在，搬迁未完成"
      exit 7
    fi
  done

  pk="$(query "SELECT string_agg(a.attname, ',' ORDER BY a.attnum) FROM pg_index i JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=ANY(i.indkey) WHERE i.indrelid='task.service_execution_ownership'::regclass AND i.indisprimary")"
  if [ "$pk" != "service_id" ]; then
    echo "ownership 主键校验失败: $pk"
    exit 8
  fi
  idx="$(query "SELECT count(*) FROM pg_indexes WHERE schemaname='task' AND indexname IN ('episode_artifact_refs_version','episode_fault_document_refs_version')")"
  if [ "$idx" != "2" ]; then
    echo "产物版本唯一索引校验失败: $idx/2"
    exit 9
  fi
  fk_cross="$(query "SELECT count(*) FROM pg_constraint WHERE contype='f' AND connamespace='task'::regnamespace AND confrelid::regclass::text NOT LIKE 'task.%'")"
  if [ "$fk_cross" != "0" ]; then
    echo "跨 schema FK 残留: $fk_cross"
    exit 10
  fi

  write_state P5 complete "$(basename "$SNAPSHOT_FILE")"
  echo "P5 搬迁完成：tasks=$after_tasks ownership=$after_ownership artifacts=$after_artifacts docs=$after_docs"
  echo "状态文件：$STATE_FILE（stage=P5 phase=complete）"
  exit 0
fi

if [ "$STAGE" = "P4" ]; then
  if [ -f "$STATE_FILE" ] && grep -q '"stage":"P4"' "$STATE_FILE" && grep -q '"phase":"complete"' "$STATE_FILE"; then
    echo "P4 搬迁已完成（读状态文件，no-op）"
    exit 0
  fi
  ensure_db

  for t in episodes episode_input_versions episode_app_events; do
    if [ "$(query "SELECT to_regclass('episode.${t}') IS NOT NULL")" != "t" ]; then
      echo "episode.${t} 不存在：请先执行 episode-svc -migrate（P4.1）"
      exit 5
    fi
    rows="$(query "SELECT count(*) FROM episode.${t}")"
    if [ "$rows" != "0" ]; then
      echo "episode.${t} 已含 $rows 行，拒绝覆盖"
      exit 5
    fi
  done

  SNAPSHOT_FILE="$(snapshot P4)"
  echo "S0 预检完成：$SNAPSHOT_FILE"

  before_episodes="$(query "SELECT count(*) FROM system.episodes")"
  before_inputs="$(query "SELECT count(*) FROM system.episode_input_versions")"
  before_app="$(query "SELECT count(*) FROM system.episode_app_events")"

  MIGRATE_SQL="$(cat <<'SQL'
ALTER TABLE system.episodes DROP CONSTRAINT IF EXISTS episodes_service_id_fkey;
ALTER TABLE system.episode_input_versions DROP CONSTRAINT IF EXISTS episode_input_versions_episode_id_fkey;
ALTER TABLE system.episode_app_events DROP CONSTRAINT IF EXISTS episode_app_events_service_id_fkey;
ALTER TABLE system.episode_app_events DROP CONSTRAINT IF EXISTS episode_app_events_episode_id_fkey;

-- 目标域为已迁移的空表；删除以让 SET SCHEMA 接管（索引/identity 序列随表迁移）。
DROP TABLE IF EXISTS episode.episode_app_events CASCADE;
DROP TABLE IF EXISTS episode.episode_input_versions CASCADE;
DROP TABLE IF EXISTS episode.episodes CASCADE;

ALTER TABLE system.episodes SET SCHEMA episode;
ALTER TABLE system.episode_input_versions SET SCHEMA episode;
ALTER TABLE system.episode_app_events SET SCHEMA episode;

ALTER TABLE episode.episode_input_versions
  ADD CONSTRAINT episode_input_versions_episode_id_fkey
  FOREIGN KEY (episode_id) REFERENCES episode.episodes(episode_id) ON DELETE RESTRICT;
ALTER TABLE episode.episode_app_events
  ADD CONSTRAINT episode_app_events_episode_id_fkey
  FOREIGN KEY (episode_id) REFERENCES episode.episodes(episode_id) ON DELETE RESTRICT;
SQL
)"

  if [ "$DRY_RUN" = true ]; then
    printf '%s\n' "$MIGRATE_SQL"
    echo "[dry-run] P4 搬迁 SQL 打印完毕（未执行）"
    exit 0
  fi
  if [ "$CONFIRM" != true ]; then
    echo "拒绝执行危险动作（缺少 --confirm）"
    exit 4
  fi

  printf '%s\n' "$MIGRATE_SQL" | psql_exec --single-transaction >/dev/null

  psql_exec >/dev/null <<'SQL'
GRANT ALL ON episode.episodes TO alert_episode;
GRANT ALL ON episode.episode_input_versions TO alert_episode;
GRANT ALL ON episode.episode_app_events TO alert_episode;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA episode TO alert_episode;
SQL

  after_episodes="$(query "SELECT count(*) FROM episode.episodes")"
  after_inputs="$(query "SELECT count(*) FROM episode.episode_input_versions")"
  after_app="$(query "SELECT count(*) FROM episode.episode_app_events")"
  if [ "$before_episodes" != "$after_episodes" ] || [ "$before_inputs" != "$after_inputs" ] || [ "$before_app" != "$after_app" ]; then
    echo "行数对账失败: episodes $before_episodes->$after_episodes, inputs $before_inputs->$after_inputs, app_events $before_app->$after_app"
    exit 6
  fi
  for t in episodes episode_input_versions episode_app_events; do
    if [ "$(query "SELECT to_regclass('system.${t}') IS NOT NULL")" = "t" ]; then
      echo "system.${t} 仍存在，搬迁未完成"
      exit 7
    fi
  done

  idx_count="$(query "SELECT count(*) FROM pg_indexes WHERE schemaname='episode' AND indexname IN ('episodes_one_active','episodes_service_idx','episodes_status_idx','app_events_service_idx','app_events_episode_idx')")"
  if [ "$idx_count" != "5" ]; then
    echo "索引校验失败: episode 域仅剩 $idx_count/5 个预期索引"
    exit 8
  fi
  fk_count="$(query "SELECT count(*) FROM pg_constraint WHERE contype='f' AND connamespace='episode'::regnamespace")"
  if [ "$fk_count" != "2" ]; then
    echo "同域外键校验失败: episode 域 FK=$fk_count（预期 2，跨域 FK 应为 0）"
    exit 9
  fi

  write_state P4 complete "$(basename "$SNAPSHOT_FILE")"
  echo "P4 搬迁完成：episodes=$after_episodes inputs=$after_inputs app_events=$after_app"
  echo "状态文件：$STATE_FILE（stage=P4 phase=complete）"
  exit 0
fi

if [ "$STAGE" = "P3" ]; then
  if [ -f "$STATE_FILE" ] && grep -q '"stage":"P3"' "$STATE_FILE" && grep -q '"phase":"complete"' "$STATE_FILE"; then
    echo "P3 搬迁已完成（读状态文件，no-op）"
    exit 0
  fi
  ensure_db

  if [ "$(query "SELECT to_regclass('monitor.service_monitor_points') IS NOT NULL")" != "t" ]; then
    echo "monitor.service_monitor_points 不存在：请先执行 monitor-svc -migrate（P3.1）"
    exit 5
  fi
  monitor_rows="$(query "SELECT count(*) FROM monitor.service_monitor_points")"
  if [ "$monitor_rows" != "0" ]; then
    echo "monitor.service_monitor_points 已含 $monitor_rows 行，拒绝覆盖"
    exit 5
  fi

  SNAPSHOT_FILE="$(snapshot P3)"
  echo "S0 预检完成：$SNAPSHOT_FILE"
  before_points="$(query "SELECT count(*) FROM system.service_monitor_points")"

  MIGRATE_SQL="$(cat <<'SQL'
ALTER TABLE system.service_monitor_points DROP CONSTRAINT IF EXISTS service_monitor_points_service_id_fkey;
ALTER TABLE system.service_monitor_points DROP CONSTRAINT IF EXISTS service_monitor_points_target_id_fkey;
-- 前置校验已保证 monitor 版为空表；删空表以让 SET SCHEMA 接管（保留索引/序列随表迁移）。
DROP TABLE IF EXISTS monitor.service_monitor_points;
ALTER TABLE system.service_monitor_points SET SCHEMA monitor;
SQL
)"

  if [ "$DRY_RUN" = true ]; then
    printf '%s\n' "$MIGRATE_SQL"
    echo "[dry-run] P3 搬迁 SQL 打印完毕（未执行）"
    exit 0
  fi
  if [ "$CONFIRM" != true ]; then
    echo "拒绝执行危险动作（缺少 --confirm）"
    exit 4
  fi

  printf '%s\n' "$MIGRATE_SQL" | psql_exec --single-transaction >/dev/null

  psql_exec >/dev/null <<'SQL'
GRANT ALL ON monitor.service_monitor_points TO alert_monitor;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA monitor TO alert_monitor;
SQL

  after_points="$(query "SELECT count(*) FROM monitor.service_monitor_points")"
  if [ "$before_points" != "$after_points" ]; then
    echo "行数对账失败: points $before_points->$after_points"
    exit 6
  fi
  if [ "$(query "SELECT to_regclass('system.service_monitor_points') IS NOT NULL")" = "t" ]; then
    echo "system.service_monitor_points 仍存在，搬迁未完成"
    exit 7
  fi

  write_state P3 complete "$(basename "$SNAPSHOT_FILE")"
  echo "P3 搬迁完成：points=$after_points"
  echo "状态文件：$STATE_FILE（stage=P3 phase=complete）"
  exit 0
fi

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

  CHECKSUM="$(sha256sum "$ROOT/go/services/service-registry/migrations/0001_registry_core.sql" | awk '{print $1}')"
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