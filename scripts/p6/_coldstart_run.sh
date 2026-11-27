#!/usr/bin/env bash
set +e
cd /home/sancho/workspace/projects/Alert || exit 9
export COMPOSE_PROFILES=core
C=deploy/compose/compose.yaml
LOG=docs/evidence/p6/cold-start.log
mkdir -p docs/evidence/p6 docs/evidence/p6/routes
exec > >(tee "$LOG") 2>&1
echo "===== P6.7 cold start $(date -u +%FT%TZ) HEAD=$(git rev-parse --short HEAD) ====="

wait_health() { # name maxsec
  local c=$1 max=${2:-180} i=0
  while [ $i -lt $max ]; do
    s=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$c" 2>/dev/null)
    [ "$s" = healthy ] && { echo "  $c healthy (${i}s)"; return 0; }
    [ "$s" = exited ] && { echo "  $c exited"; return 1; }
    sleep 2; i=$((i+2))
  done
  echo "  $c NOT healthy after ${max}s"; docker logs --tail 50 "$c" 2>&1; return 1
}

echo "== S1 down -v =="; docker compose -f $C down -v --remove-orphans 2>&1 | tail -3
echo "== S2 postgres+kafka =="; docker compose -f $C up -d postgres kafka 2>&1 | tail -2
wait_health alert-pg 180; wait_health alert-kafka 180
echo "== S3 kafka-init =="; docker compose -f $C run --rm kafka-init 2>&1 | tail -5
echo "== S4 provision =="; docker compose -f $C --profile setup run --rm provision 2>&1 | tail -5
echo "== S5 migrate 4 domains =="
for s in service-registry monitor-svc episode-svc task-orchestrator; do
  echo "-- $s -migrate --"
  docker compose -f $C --profile core run --rm "$s" -migrate 2>&1 | tail -3
done
echo "== S6 agent cognitive migrate =="
docker compose -f $C --profile core run --rm agent-runtime python -m alert_agent.bootstrap.migrate 2>&1 | tail -5
echo "== S7 up core =="
docker compose -f $C up -d 2>&1 | tail -5
echo "-- wait registry/bff, then bootstrap catalog event for monitor readyz --"
wait_health alert-service-registry 180
wait_health alert-console-bff 180
docker exec alert-service-registry wget -qO- --header='Content-Type: application/json' \
  --post-data='{"service_key":"bootstrap-svc","name":"Bootstrap","description":"bootstrap","authority_producer_id":"bootstrap"}' \
  http://127.0.0.1:8081/internal/services >/dev/null 2>&1 && echo "bootstrap service emitted" || echo "bootstrap emit skipped"
for c in alert-service-registry alert-monitor-svc alert-episode-svc alert-task-orchestrator alert-console-bff alert-agent-runtime; do wait_health "$c" 240; done
echo "== ps =="
for c in alert-pg alert-kafka alert-service-registry alert-monitor-svc alert-episode-svc alert-task-orchestrator alert-console-bff alert-agent-runtime; do echo "$c=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' $c 2>/dev/null)"; done | tee docs/evidence/p6/ps.txt
echo "kafka-init=$(docker inspect -f '{{.State.Status}} exit={{.State.ExitCode}}' alert-kafka-init 2>/dev/null)" | tee -a docs/evidence/p6/ps.txt
echo -n "readyz="; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/readyz

echo "== S8 snapshot 18 =="
OUT_DIR=docs/evidence/p6/routes bash scripts/snapshot-routes.sh 2>&1 | tail -3
echo "-- single-resource diffs (D-23) --"
for f in 03_service_create 04_service_get 05_service_patch 06_service_delete 07_targets_list 08_target_create 09_target_patch 10_target_delete 12_episode_get 13_app_event 14_service_timeline 15_episode_timeline 16_artifacts_get 17_retry_diagnosis 18_retry_archive 19_err_not_found 20_err_conflict 21_err_validation; do
  diff -q "docs/evidence/p0/routes/$f.json" "docs/evidence/p6/routes/$f.json" >/dev/null 2>&1 && echo "OK $f" || echo "DIFF $f"
done
echo "-- list/agg shape --"
jq -S 'keys' docs/evidence/p6/routes/01_dashboard_summary.json > /tmp/p6-01.json; jq -S 'keys' docs/evidence/p0/routes/01_dashboard_summary.json > /tmp/p0-01.json; diff /tmp/p0-01.json /tmp/p6-01.json >/dev/null && echo "01 shape OK" || echo "01 shape DIFF"
jq -S '[.services[]|keys]|.[0]' docs/evidence/p6/routes/02_services_list.json > /tmp/p6-02.json; jq -S '[.services[]|keys]|.[0]' docs/evidence/p0/routes/02_services_list.json > /tmp/p0-02.json; diff /tmp/p0-02.json /tmp/p6-02.json >/dev/null && echo "02 shape OK" || echo "02 shape DIFF"
jq -S '[.episodes[]|keys]|.[0]' docs/evidence/p6/routes/11_episodes_list.json > /tmp/p6-11.json; jq -S '[.episodes[]|keys]|.[0]' docs/evidence/p0/routes/11_episodes_list.json > /tmp/p0-11.json; diff /tmp/p0-11.json /tmp/p6-11.json >/dev/null && echo "11 shape OK" || echo "11 shape DIFF"

echo "== S10 isolation =="; bash deploy/split/verify-isolation.sh 2>&1 | tail -3 | tee docs/evidence/p6/isolation.txt
echo "== S11 structure =="; bash scripts/verify-p7.sh --structure-only 2>&1 | tail -6 | tee docs/evidence/p6/structure.txt
echo "== S12 mem =="; docker stats --no-stream --format '{{.Name}}\t{{.MemUsage}}' | tee docs/evidence/p6/mem.tsv
echo "== S13 minimal topology demo =="
docker compose -f $C down 2>&1 | tail -2
docker compose -f $C up -d 2>&1 | tail -2
for c in alert-service-registry alert-monitor-svc alert-episode-svc alert-task-orchestrator alert-console-bff alert-agent-runtime; do wait_health "$c" 240; done
bash scripts/demo.sh --minimal 2>&1 | tail -12 | tee docs/evidence/p6/minimal.log; echo "minimal_exit=${PIPESTATUS[0]}"
echo "===== DONE $(date -u +%FT%TZ) ====="