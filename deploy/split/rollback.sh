#!/usr/bin/env bash
# 阶段回退骨架：--to P0 实装 R1（停新服务）/R5（恢复旧进程）；
# P2~P5 的 R2~R4 尚未实现，显式返回非零，禁止"假回退"。
set -euo pipefail

SPLIT_DIR="$(cd "$(dirname "$0")" && pwd)"
STATE_FILE="$SPLIT_DIR/.split-state.json"
PG_CONTAINER="${PG_CONTAINER:-alert-pg}"
PGDATABASE="${PGDATABASE:-alert}"
COMPOSE_FILE="$SPLIT_DIR/../compose/compose.yaml"

TARGET="P0"
DRY_RUN=false
CONFIRM=false
RESTORE=false
while [ $# -gt 0 ]; do
  case "$1" in
    --to) TARGET="${2:?--to 需要值}"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    --confirm) CONFIRM=true; shift ;;
    --restore) RESTORE=true; shift ;;
    *) echo "未知参数: $1"; exit 2 ;;
  esac
done

if [ -f "$STATE_FILE" ]; then
  state="$(cat "$STATE_FILE")"
else
  state='{"stage":"P0","phase":"not-started","started_at":"","snapshot":""}'
fi
echo "当前状态：$state"

run_or_echo() {
  local desc="$1"
  shift
  if [ "$DRY_RUN" = true ]; then
    echo "[dry-run] $desc: $*"
    return 0
  fi
  if [ "$CONFIRM" != true ]; then
    echo "拒绝执行危险动作（缺少 --confirm）：$desc"
    exit 4
  fi
  echo "[执行] $desc"
  "$@"
}


# P2 回退：registry 三表移回 system，重建 9 条跨域 FK，并清 registry 版本行。
rollback_registry_tables() {
  docker exec -i -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
    -U alert -d "$PGDATABASE" --set=ON_ERROR_STOP=1 --single-transaction <<'SQL'
ALTER TABLE registry.service_monitor_targets DROP CONSTRAINT IF EXISTS service_monitor_targets_service_id_fkey;
ALTER TABLE registry.service_source_states DROP CONSTRAINT IF EXISTS service_source_states_service_id_fkey;
ALTER TABLE registry.services SET SCHEMA system;
ALTER TABLE registry.service_monitor_targets SET SCHEMA system;
ALTER TABLE registry.service_source_states SET SCHEMA system;
ALTER TABLE system.episodes ADD CONSTRAINT episodes_service_id_fkey FOREIGN KEY (service_id) REFERENCES system.services(service_id) ON DELETE RESTRICT;
ALTER TABLE system.service_monitor_points ADD CONSTRAINT service_monitor_points_service_id_fkey FOREIGN KEY (service_id) REFERENCES system.services(service_id) ON DELETE RESTRICT;
ALTER TABLE system.service_monitor_points ADD CONSTRAINT service_monitor_points_target_id_fkey FOREIGN KEY (target_id) REFERENCES system.service_monitor_targets(target_id) ON DELETE RESTRICT;
ALTER TABLE system.episode_app_events ADD CONSTRAINT episode_app_events_service_id_fkey FOREIGN KEY (service_id) REFERENCES system.services(service_id) ON DELETE RESTRICT;
ALTER TABLE system.agent_tasks ADD CONSTRAINT agent_tasks_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES system.episodes(episode_id) ON DELETE RESTRICT;
ALTER TABLE system.agent_tasks ADD CONSTRAINT agent_tasks_service_id_fkey FOREIGN KEY (service_id) REFERENCES system.services(service_id) ON DELETE RESTRICT;
ALTER TABLE system.service_execution_ownership ADD CONSTRAINT service_execution_ownership_service_id_fkey FOREIGN KEY (service_id) REFERENCES system.services(service_id) ON DELETE RESTRICT;
ALTER TABLE system.episode_artifact_refs ADD CONSTRAINT episode_artifact_refs_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES system.episodes(episode_id) ON DELETE RESTRICT;
ALTER TABLE system.episode_fault_document_refs ADD CONSTRAINT episode_fault_document_refs_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES system.episodes(episode_id) ON DELETE RESTRICT;
DELETE FROM registry.schema_migrations WHERE version=1;
SQL
  local tmp
  tmp="$(mktemp "$SPLIT_DIR/.split-state.json.XXXXXX")"
  printf '{"stage":"P0","phase":"rolled-back","started_at":"%s","snapshot":""}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp"
  mv "$tmp" "$STATE_FILE"
}

# P3 回退：points 移回 system。注意 P2 完成态本身不含跨域 FK（S1 已拆），
# 因此这里只做 SET SCHEMA；若要恢复完整单体（含 FK）请使用 --to P0。
rollback_monitor_points() {
  docker exec -i -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
    -U alert -d "$PGDATABASE" --set=ON_ERROR_STOP=1 --single-transaction <<'SQL'
ALTER TABLE monitor.service_monitor_points SET SCHEMA system;
GRANT ALL ON system.service_monitor_points TO alert;
SQL
  local tmp
  tmp="$(mktemp "$SPLIT_DIR/.split-state.json.XXXXXX")"
  printf '{"stage":"P2","phase":"rolled-back","started_at":"%s","snapshot":""}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp"
  mv "$tmp" "$STATE_FILE"
}

# P4 回退：episode 三表移回 system（P4.2 的逆操作；跨域 FK 不恢复，与 P3 完成态一致）。
rollback_episode_tables() {
  docker exec -i -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
    -U alert -d "$PGDATABASE" --set=ON_ERROR_STOP=1 --single-transaction <<'SQL'
ALTER TABLE episode.episode_input_versions DROP CONSTRAINT IF EXISTS episode_input_versions_episode_id_fkey;
ALTER TABLE episode.episode_app_events DROP CONSTRAINT IF EXISTS episode_app_events_episode_id_fkey;
ALTER TABLE episode.episodes SET SCHEMA system;
ALTER TABLE episode.episode_input_versions SET SCHEMA system;
ALTER TABLE episode.episode_app_events SET SCHEMA system;
ALTER TABLE system.episode_input_versions ADD CONSTRAINT episode_input_versions_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES system.episodes(episode_id) ON DELETE RESTRICT;
ALTER TABLE system.episode_app_events ADD CONSTRAINT episode_app_events_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES system.episodes(episode_id) ON DELETE RESTRICT;
SQL
  local tmp
  tmp="$(mktemp "$SPLIT_DIR/.split-state.json.XXXXXX")"
  printf '{"stage":"P3","phase":"rolled-back","started_at":"%s","snapshot":""}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp"
  mv "$tmp" "$STATE_FILE"
}

# P5 回退：task 四表移回 system（P5.2 的逆操作；同域 FK 保留）。
rollback_task_tables() {
  docker exec -i -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql \
    -U alert -d "$PGDATABASE" --set=ON_ERROR_STOP=1 --single-transaction <<'SQL'
ALTER TABLE task.service_execution_ownership DROP CONSTRAINT IF EXISTS service_execution_ownership_task_id_fkey;
ALTER TABLE task.episode_artifact_refs DROP CONSTRAINT IF EXISTS episode_artifact_refs_task_id_fkey;
ALTER TABLE task.episode_fault_document_refs DROP CONSTRAINT IF EXISTS episode_fault_document_refs_task_id_fkey;
ALTER TABLE task.episode_fault_document_refs SET SCHEMA system;
ALTER TABLE task.episode_artifact_refs SET SCHEMA system;
ALTER TABLE task.service_execution_ownership SET SCHEMA system;
ALTER TABLE task.agent_tasks SET SCHEMA system;
ALTER TABLE system.service_execution_ownership ADD CONSTRAINT service_execution_ownership_task_id_fkey FOREIGN KEY (task_id) REFERENCES system.agent_tasks(task_id) ON DELETE RESTRICT;
ALTER TABLE system.episode_artifact_refs ADD CONSTRAINT episode_artifact_refs_task_id_fkey FOREIGN KEY (task_id) REFERENCES system.agent_tasks(task_id) ON DELETE RESTRICT;
ALTER TABLE system.episode_fault_document_refs ADD CONSTRAINT episode_fault_document_refs_task_id_fkey FOREIGN KEY (task_id) REFERENCES system.agent_tasks(task_id) ON DELETE RESTRICT;
SQL
}

case "$TARGET" in
  P1)
    # P1 回退：停 BFF + 控制台指回单体；--restore 恢复 P1 链路。
    NGINX_FILE="$SPLIT_DIR/../../apps/console/nginx.conf"
    if [ "$RESTORE" = true ]; then
      run_or_echo "R1' 启动 BFF" docker compose -f "$COMPOSE_FILE" up -d console-bff
      run_or_echo "R2' 控制台指回 BFF" sed -i 's|http://system-core:8080|http://console-bff:8080|' "$NGINX_FILE"
    else
      run_or_echo "R1 停 BFF" docker compose -f "$COMPOSE_FILE" stop console-bff
      run_or_echo "R2 控制台指回单体" sed -i 's|http://console-bff:8080|http://system-core:8080|' "$NGINX_FILE"
    fi
    ;;
  P0)
    # R1：P0 尚无新服务，记录为 no-op。
    if [ "$(docker exec -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql -U alert -d "$PGDATABASE" -tAc "SELECT to_regclass('registry.services') IS NOT NULL" 2>/dev/null)" = "t" ]; then
      run_or_echo "R2 反向移表 registry→system" rollback_registry_tables
    fi
    echo "R1 停新服务：P0 无新服务（no-op）"
    # R5：恢复旧进程（system-core api/worker），由操作者显式启动。
    if [ "$DRY_RUN" = true ]; then
      echo "[dry-run] R5 恢复旧进程：go run ./go/services/system-core/cmd/api 与 cmd/worker"
    else
      echo "R5 恢复旧进程：请按 README 启动 system-core api/worker（脚本不自动拉起裸进程）"
    fi
    ;;
  P2)
    if [ "$(docker exec -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql -U alert -d "$PGDATABASE" -tAc "SELECT to_regclass('monitor.service_monitor_points') IS NOT NULL" 2>/dev/null)" = "t" ]; then
      run_or_echo "R2 反向移表 monitor→system" rollback_monitor_points
    fi
    echo "R1 停 monitor-svc：P3 前 compose 未启用（no-op）"
    ;;
  P3)
    if [ "$(docker exec -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql -U alert -d "$PGDATABASE" -tAc "SELECT to_regclass('task.agent_tasks') IS NOT NULL" 2>/dev/null)" = "t" ]; then
      run_or_echo "R1 停 task-orchestrator" docker rm -f alert-task-orchestrator
      run_or_echo "R2 反向移表 task→system" rollback_task_tables
    fi
    if [ "$(docker exec -e PGPASSWORD="${PGPASSWORD:-alert}" "$PG_CONTAINER" psql -U alert -d "$PGDATABASE" -tAc "SELECT to_regclass('episode.episodes') IS NOT NULL" 2>/dev/null)" = "t" ]; then
      run_or_echo "R3 停 episode-svc" docker rm -f alert-episode-svc
      run_or_echo "R4 反向移表 episode→system" rollback_episode_tables
    fi
    run_or_echo "R5 BFF 回 compose.p3（episode 路由指单体）" docker compose -f "$SPLIT_DIR/../compose/compose.p3.yaml" up -d --no-deps console-bff
    echo "R6 代码回退：git revert P4.5..P4.9（P4.8 已删单体 episode 直读，运行态回退不能恢复该链路）"
    ;;
  P4|P5)
    echo "未实现阶段：$TARGET 的 R2~R4 尚未实装（P5.11 内补齐 --to P4）"
    exit 3
    ;;
  *)
    echo "未知目标阶段：$TARGET（支持 P0|P1|P2|P3|P4|P5）"
    exit 2
    ;;
esac

echo "rollback: $TARGET 完成（dry_run=$DRY_RUN restore=$RESTORE）"