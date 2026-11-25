#!/usr/bin/env bash
# 固定 UUID/时间的演示数据：2 服务 / 3 目标 / 1 ACTIVE episode / 1 SUCCEEDED 任务 / 2 产物引用。
# --reset 先清理同一批固定 ID 再写入；--dry-run 只打印 SQL。
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-alert-pg}"
PGDATABASE="${PGDATABASE:-alert}"
RESET=false
DRY_RUN=false
while [ $# -gt 0 ]; do
  case "$1" in
    --reset) RESET=true; shift ;;
    --dry-run) DRY_RUN=true; shift ;;
    *) echo "未知参数: $1"; exit 2 ;;
  esac
done

SVC_1="11111111-1111-1111-1111-111111111111"
SVC_2="11111111-1111-1111-1111-111111111112"
TGT_1="22222222-2222-2222-2222-222222222222"
TGT_2="22222222-2222-2222-2222-222222222223"
TGT_3="22222222-2222-2222-2222-222222222224"
EP_1="33333333-3333-3333-3333-333333333333"
TASK_1="44444444-4444-4444-4444-444444444444"
ART_1="55555555-5555-5555-5555-555555555555"
ART_2="55555555-5555-5555-5555-555555555556"
TS="2026-11-01T00:00:00Z"

SQL="$(cat <<SQL
BEGIN;

$(if [ "$RESET" = true ]; then cat <<RESET_SQL
DELETE FROM system.episode_artifact_refs WHERE episode_id = '${EP_1}';
DELETE FROM system.episode_fault_document_refs WHERE episode_id = '${EP_1}';
DELETE FROM system.service_execution_ownership WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.agent_tasks WHERE episode_id = '${EP_1}';
DELETE FROM system.episode_input_versions WHERE episode_id = '${EP_1}';
DELETE FROM system.episode_app_events WHERE episode_id = '${EP_1}';
DELETE FROM system.episode_input_versions
  WHERE episode_id IN (SELECT episode_id FROM system.episodes WHERE service_id IN ('${SVC_1}','${SVC_2}'));
DELETE FROM system.episode_app_events WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.service_monitor_points WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.episode_input_versions
  WHERE episode_id IN (SELECT episode_id FROM system.episodes WHERE service_id IN ('${SVC_1}','${SVC_2}'));
DELETE FROM system.episode_app_events WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.service_monitor_points WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.episodes WHERE episode_id = '${EP_1}';
DELETE FROM system.service_monitor_targets WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.service_source_states WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.services WHERE service_id IN ('${SVC_1}','${SVC_2}');
DELETE FROM system.episode_input_versions
  WHERE episode_id IN (SELECT episode_id FROM system.episodes
    WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created')));
DELETE FROM system.episode_app_events
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.service_monitor_points
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.episode_input_versions
  WHERE episode_id IN (SELECT episode_id FROM system.episodes
    WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created')));
DELETE FROM system.episode_app_events
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.service_monitor_points
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.service_monitor_targets
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.service_source_states
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.episodes
  WHERE service_id IN (SELECT service_id FROM system.services WHERE service_key IN ('snapshot-created','smoke-created'));
DELETE FROM system.services WHERE service_key IN ('snapshot-created','smoke-created');
RESET_SQL
fi)

INSERT INTO system.services
  (service_id, service_key, name, description, enabled, monitor_aggregation_mode, authority_producer_id, created_at, updated_at)
VALUES
  ('${SVC_1}', 'checkout-api', 'Checkout API', 'baseline checkout service', true, 'ALL_REQUIRED', 'producer-checkout', '${TS}', '${TS}'),
  ('${SVC_2}', 'payment-api', 'Payment API', 'baseline payment service', true, 'ALL_REQUIRED', 'producer-payment', '${TS}', '${TS}');

INSERT INTO system.service_monitor_targets
  (target_id, service_id, target_key, target_name, address, target_type, config, interval_ms, timeout_ms,
   failure_threshold, recovery_threshold, is_required, enabled, status, consecutive_failures,
   consecutive_successes, last_check_at, config_generation, created_at, updated_at)
VALUES
  ('${TGT_1}', '${SVC_1}', 'http-primary', 'Checkout primary', 'http://127.0.0.1:18081/healthz', 'HTTP', '{}'::jsonb,
   15000, 3000, 2, 2, true, true, 'NORMAL', 0, 3, '${TS}', 1, '${TS}', '${TS}'),
  ('${TGT_2}', '${SVC_1}', 'http-secondary', 'Checkout secondary', 'http://127.0.0.1:18082/healthz', 'HTTP', '{}'::jsonb,
   15000, 3000, 2, 2, false, true, 'ABNORMAL', 2, 0, '${TS}', 1, '${TS}', '${TS}'),
  ('${TGT_3}', '${SVC_2}', 'tcp-primary', 'Payment TCP', '127.0.0.1:18083', 'TCP', '{}'::jsonb,
   15000, 3000, 2, 2, true, true, 'NORMAL', 0, 5, '${TS}', 1, '${TS}', '${TS}');

INSERT INTO system.service_source_states
  (service_id, source_type, status, reason, updated_at, last_change_at)
VALUES
  ('${SVC_1}', 'MONITOR', 'ABNORMAL', 'baseline abnormal target', '${TS}', '${TS}'),
  ('${SVC_1}', 'APP', 'NORMAL', 'baseline app healthy', '${TS}', '${TS}'),
  ('${SVC_2}', 'MONITOR', 'NORMAL', 'baseline monitor healthy', '${TS}', '${TS}'),
  ('${SVC_2}', 'APP', 'NORMAL', 'baseline app healthy', '${TS}', '${TS}');

INSERT INTO system.episodes
  (episode_id, service_id, status, started_by, resolution_pending, input_hash, input_version,
   processed_input_version, final_input_version, monitor_watermark, app_event_watermark,
   started_at, created_at, updated_at)
VALUES
  ('${EP_1}', '${SVC_1}', 'ACTIVE', 'MONITOR', false, 'baseline-input-hash', 2,
   1, 2, 1, 0, '${TS}', '${TS}', '${TS}');

INSERT INTO system.agent_tasks
  (task_id, episode_id, service_id, task_type, status, generation, attempt, max_attempts,
   input_version_from, input_version_to, monitor_watermark, app_event_watermark, deadline_at,
   created_at, updated_at, started_at, finished_at)
VALUES
  ('${TASK_1}', '${EP_1}', '${SVC_1}', 'DIAGNOSIS', 'SUCCEEDED', 1, 1, 3,
   1, 2, 1, 0, '${TS}', '${TS}', '${TS}', '${TS}', '${TS}');

INSERT INTO system.episode_artifact_refs
  (artifact_ref_id, episode_id, task_id, generation, artifact_type, artifact_version,
   content_hash, content_type, storage_key, source_refs, registered_at, updated_at)
VALUES
  ('${ART_1}', '${EP_1}', '${TASK_1}', 1, 'INVESTIGATION', 1,
   'sha256:investigation-baseline', 'text/markdown', 'episodes/${EP_1}/investigation/1.md', '[]'::jsonb, '${TS}', '${TS}'),
  ('${ART_2}', '${EP_1}', '${TASK_1}', 1, 'DIAGNOSIS_REPORT', 1,
   'sha256:diagnosis-baseline', 'text/markdown', 'episodes/${EP_1}/diagnosis/1.md', '[]'::jsonb, '${TS}', '${TS}');

COMMIT;
SQL
)"

if [ "$DRY_RUN" = true ]; then
  printf '%s\n' "$SQL"
  echo "seed: dry-run 完成（未连接数据库）"
  exit 0
fi

printf '%s\n' "$SQL" | docker exec -i "$PG_CONTAINER" psql --username=alert --dbname="$PGDATABASE" \
  --no-psqlrc --set=ON_ERROR_STOP=1 >/dev/null

echo "seed: 完成（reset=$RESET）"