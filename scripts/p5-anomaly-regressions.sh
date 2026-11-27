#!/usr/bin/env bash
# P5.11 异常回归（真实 PG + task reconciler，非 mock）：
#   A 租约重派：过期 RUNNING → generation+1、清租约、离开 RUNNING
#   B attempt 上限：过期 RUNNING 且 attempt>=max_attempts → FAILED(LEASE_RECLAIM_EXHAUSTED)
#   C 对账补建：陈旧 DISPATCHED（久未受理）→ reconciler 触碰（updated_at 刷新、离开旧态）
#   D 旧代次拒绝：应用层 fence token，需 gRPC 构造；本脚本不覆盖（见 evidence）
set +e
cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 9
export COMPOSE_PROFILES=core
C=deploy/compose/compose.yaml
PG="docker exec -i alert-pg psql -U alert -d alert"
Q(){ $PG -tAc "$1" 2>/dev/null; }

wait_health(){ local c=$1 i=0; while [ $i -lt 90 ]; do [ "$(docker inspect -f '{{.State.Health.Status}}' $c 2>/dev/null)" = healthy ] && return 0; sleep 2; i=$((i+2)); done; return 1; }

cleanup(){
  $PG >/dev/null 2>&1 <<'SQL'
DELETE FROM task.service_execution_ownership WHERE task_id IN
 ('7a000001-0000-0000-0000-000000000001','7a000002-0000-0000-0000-000000000002','7a000003-0000-0000-0000-000000000003');
DELETE FROM task.agent_tasks WHERE task_id IN
 ('7a000001-0000-0000-0000-000000000001','7a000002-0000-0000-0000-000000000002','7a000003-0000-0000-0000-000000000003');
SQL
}

echo "== rebuild/restart task-orchestrator (ensure new ReclaimExpired) =="
docker compose -f $C up -d --build task-orchestrator >/dev/null 2>&1
wait_health alert-task-orchestrator || { echo "task not healthy"; exit 2; }

cleanup
echo "== inject fixtures =="
$PG >/dev/null 2>&1 <<'SQL'
INSERT INTO task.agent_tasks
 (task_id,episode_id,service_id,task_type,status,generation,attempt,max_attempts,lease_until,next_retry_at,
  input_version_from,input_version_to,monitor_watermark,app_event_watermark,deadline_at,error_code,error_message,created_at,updated_at)
VALUES
 ('7a000001-0000-0000-0000-000000000001','7e000001-0000-0000-0000-000000000001','7c000001-0000-0000-0000-000000000001','DIAGNOSIS','RUNNING',1,1,3, now()-interval '2 minute', NULL, 1,1,0,0, now()+interval '1 hour','','REG_LEASE', now(), now()),
 ('7a000002-0000-0000-0000-000000000002','7e000002-0000-0000-0000-000000000002','7c000002-0000-0000-0000-000000000002','DIAGNOSIS','RUNNING',1,3,3, now()-interval '2 minute', NULL, 1,1,0,0, now()+interval '1 hour','','REG_ATTEMPT', now(), now()),
 ('7a000003-0000-0000-0000-000000000003','7e000003-0000-0000-0000-000000000003','7c000003-0000-0000-0000-000000000003','DIAGNOSIS','DISPATCHED',1,1,3, NULL, NULL, 1,1,0,0, now()+interval '1 hour','','REG_STALE', now(), now()-interval '10 minute');
SQL

echo "== wait reconciler (<=30s) =="; sleep 45

A_id=7a000001-0000-0000-0000-000000000001; B_id=7a000002-0000-0000-0000-000000000002; C_id=7a000003-0000-0000-0000-000000000003
A_status=$(Q "SELECT status FROM task.agent_tasks WHERE task_id='$A_id'")
A_gen=$(Q "SELECT generation FROM task.agent_tasks WHERE task_id='$A_id'")
A_lease_null=$(Q "SELECT (lease_until IS NULL) FROM task.agent_tasks WHERE task_id='$A_id'")
B_status=$(Q "SELECT status FROM task.agent_tasks WHERE task_id='$B_id'")
B_code=$(Q "SELECT error_code FROM task.agent_tasks WHERE task_id='$B_id'")
B_gen=$(Q "SELECT generation FROM task.agent_tasks WHERE task_id='$B_id'")
C_status=$(Q "SELECT status FROM task.agent_tasks WHERE task_id='$C_id'")
C_recent=$(Q "SELECT (updated_at > now()-interval '5 minute') FROM task.agent_tasks WHERE task_id='$C_id'")

echo "A: gen=$A_gen status=$A_status lease_null=$A_lease_null"
echo "B: gen=$B_gen status=$B_status code=$B_code"
echo "C: status=$C_status recent=$C_recent"

PASS=0; FAIL=0
ok(){ PASS=$((PASS+1)); echo "PASS $1"; }
ng(){ FAIL=$((FAIL+1)); echo "FAIL $1 :: $2"; }

{ [ "${A_gen:-0}" -ge 2 ] && [ "$A_lease_null" = "t" ] && { [ "$A_status" = "RETRY_WAIT" ] || [ "$A_status" = "DISPATCHED" ]; }; } \
  && ok "A_lease_requeue" || ng "A_lease_requeue" "gen=$A_gen status=$A_status lease_null=$A_lease_null"
{ [ "${B_gen:-0}" -ge 2 ] && [ "$B_status" = "FAILED" ] && [ "$B_code" = "LEASE_RECLAIM_EXHAUSTED" ]; } \
  && ok "B_attempt_exhausted" || ng "B_attempt_exhausted" "gen=$B_gen status=$B_status code=$B_code"
{ [ "$C_recent" = "t" ] && { [ "$C_status" = "DISPATCHED" ] || [ "$C_status" = "RETRY_WAIT" ] || [ "$C_status" = "RUNNING" ]; }; } \
  && ok "C_reconcile_stale" || ng "C_reconcile_stale" "status=$C_status recent=$C_recent"

cleanup
echo "===== SUMMARY PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = 0 ] && exit 0 || exit 1