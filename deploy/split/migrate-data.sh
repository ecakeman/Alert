#!/usr/bin/env bash
# S0 预检：记录 13 张表行数/体积、迁移版本与活跃写入，输出快照并写状态文件。
# 本脚本在 P0 只读，不移动任何表；--dry-run 与正式模式执行相同的只读查询。
set -euo pipefail

SPLIT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SPLIT_DIR/../.." && pwd)"
PG_CONTAINER="${PG_CONTAINER:-alert-pg}"
PGDATABASE="${PGDATABASE:-alert}"
STATE_FILE="$SPLIT_DIR/.split-state.json"

DRY_RUN=false
STAGE="P0"
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --stage) STAGE="${2:?--stage 需要值}"; shift 2 ;;
    *) echo "未知参数: $1"; exit 2 ;;
  esac
done

if ! docker exec "$PG_CONTAINER" psql --username=alert --dbname="$PGDATABASE" \
    --no-psqlrc --tuples-only --no-align --command="SELECT 1" >/dev/null 2>&1; then
  echo "S0 未执行（环境：Docker daemon 或 $PG_CONTAINER 不可用）"
  exit 1
fi

query() {
  docker exec "$PG_CONTAINER" psql --username=alert --dbname="$PGDATABASE" \
    --no-psqlrc --tuples-only --no-align --command="$1"
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

migration_version="$(query "SELECT coalesce(max(version),0) FROM system.schema_migrations")"
unpublished_outbox="$(query "SELECT count(*) FROM system.outbox_events WHERE published_at IS NULL")"
recent_writes="$(query "SELECT count(*) FROM system.outbox_events WHERE created_at > now() - interval '5 minutes'")"

mkdir -p "$SPLIT_DIR/snapshots"
generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
snapshot_file="$SPLIT_DIR/snapshots/$(date -u +%Y%m%dT%H%M%SZ).json"

{
  printf '{\n'
  printf '  "stage": "%s",\n' "$STAGE"
  printf '  "generated_at": "%s",\n' "$generated_at"
  printf '  "migration_version": %s,\n' "$migration_version"
  printf '  "active_writes": {"unpublished_outbox": %s, "recent_5m_outbox": %s},\n' \
    "$unpublished_outbox" "$recent_writes"
  printf '  "tables": {\n'
  last=$(( ${#TABLES[@]} - 1 ))
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

phase="s0-complete"
if [ "$DRY_RUN" = true ]; then
  phase="dry-run"
fi

state_tmp="$(mktemp "$SPLIT_DIR/.split-state.json.XXXXXX")"
printf '{"stage":"%s","phase":"%s","started_at":"%s","snapshot":"%s"}\n' \
  "$STAGE" "$phase" "$generated_at" "$(basename "$snapshot_file")" > "$state_tmp"
mv "$state_tmp" "$STATE_FILE"

echo "S0 预检完成：$snapshot_file"
echo "迁移版本=$migration_version 未发布 outbox=$unpublished_outbox 近5分钟写入=$recent_writes"
echo "状态文件：$STATE_FILE（phase=$phase）"