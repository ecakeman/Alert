#!/usr/bin/env bash
# P5.9 验收 #7：连续 N 次"派发→立即 Started"，统计被误拒（LEASE_LOST）次数。
# 前置：compose.p5.yaml 全拓扑已起；alert-agent-runtime 容器存在（含生成 stub）。
# 用法：bash scripts/p5-stress-first-callback.sh 50
set -euo pipefail
N="${1:-50}"
CONTAINER="${AGENT_CONTAINER:-alert-agent-runtime}"
TARGET="${TARGET:-task-orchestrator:9090}"

docker exec -i "$CONTAINER" python - "$N" "$TARGET" <<'PY'
import asyncio
import base64
import math
import sys

import grpc
import psycopg

from alert_agent.infrastructure.grpc.gen.alertflow.system.v1 import common_pb2, task_gateway_pb2
from alert_agent.infrastructure.grpc.gen.alertflow.system.v1.task_gateway_pb2_grpc import (
    SystemTaskGatewayServiceStub,
)

N = int(sys.argv[1])
TARGET = sys.argv[2]
DSN = "postgres://alert:alert@postgres:5432/alert?sslmode=disable"


def lease_token(task_id: str, generation: int, until_epoch: float) -> str:
    raw = f"{task_id}|{generation}|{math.ceil(until_epoch)}".encode()
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


async def main() -> int:
    with psycopg.connect(DSN) as conn:
        row = conn.execute(
            "SELECT task_id, episode_id, task_type, generation FROM task.agent_tasks "
            "WHERE task_type IN ('DIAGNOSIS','ARCHIVE') "
            "AND episode_id IS NOT NULL "
            "ORDER BY updated_at DESC LIMIT 1"
        ).fetchone()
        if row is None:
            print("no task row available", file=sys.stderr)
            return 2
        task_id, episode_id, task_type, generation = row
        conn.commit()

    denied = 0
    async with grpc.aio.insecure_channel(TARGET) as channel:
        stub = SystemTaskGatewayServiceStub(channel)
        for _ in range(N):
            with psycopg.connect(DSN) as conn:
                gen, until_epoch = conn.execute(
                    "UPDATE task.agent_tasks SET status='DISPATCHED', "
                    "lease_until=now()+interval '30 seconds', updated_at=now() "
                    "WHERE task_id=%s RETURNING generation, extract(epoch from lease_until)",
                    (task_id,),
                ).fetchone()
                conn.commit()
            identity = common_pb2.TaskIdentity(
                task_id=str(task_id),
                episode_id=str(episode_id),
                task_type=str(task_type),
                generation=int(gen),
                lease_token=lease_token(str(task_id), int(gen), float(until_epoch)),
            )
            try:
                await stub.ReportTaskStarted(
                    task_gateway_pb2.ReportTaskStartedRequest(identity=identity), timeout=5
                )
            except grpc.aio.AioRpcError as exc:
                details = exc.details() or ""
                if "LEASE_LOST" in details:
                    denied += 1
    print(f"attempts={N} denied={denied}")
    return 0 if denied == 0 else 1


raise SystemExit(asyncio.run(main()))
PY