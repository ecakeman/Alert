"""Bootstrap wiring for the persistence batch (T05).

Assembles the async pool, the Postgres receipt store, the submit use case and
the Postgres readiness probe; provides the runtime Clock/IdGen implementations.
Registering the probe into ``app.state.ready_checks`` is scheduled with the
gateway/worker batches (T07/T08) per the task plan -- this module only defines
the wiring and never touches lifespan directly.
"""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import grpc.aio
from psycopg_pool import AsyncConnectionPool

from alert_agent.application.task.ports import ReceiptStore, TaskGateway
from alert_agent.application.task.submit import SubmitTask
from alert_agent.config import Settings
from alert_agent.infrastructure.grpc.gateway_client import SystemTaskGatewayClient
from alert_agent.infrastructure.persistence.postgres.pool import build_pool
from alert_agent.infrastructure.persistence.postgres.receipt_repo import PostgresReceiptStore

__all__ = [
    "SystemClock",
    "UuidIdGen",
    "build_gateway_channel",
    "build_gateway_client",
    "build_pool",
    "build_receipt_store",
    "build_submit_use_case",
    "postgres_ready_check",
]


class SystemClock:
    """Timezone-aware UTC clock (application ``Clock`` port)."""

    def now(self) -> datetime:
        return datetime.now(UTC)


class UuidIdGen:
    """UUID4-based identifier generator (application ``IdGen`` port)."""

    def new_id(self, prefix: str) -> str:
        return f"{prefix}_{uuid4().hex}"


def build_receipt_store(pool: AsyncConnectionPool) -> ReceiptStore:
    return PostgresReceiptStore(pool)


def build_submit_use_case(pool: AsyncConnectionPool) -> SubmitTask:
    return SubmitTask(build_receipt_store(pool), SystemClock())


async def postgres_ready_check(pool: AsyncConnectionPool) -> None:
    """Real Postgres probe for /ready (CR-BOOT-08); raises on any failure."""
    async with pool.connection() as connection:
        await connection.execute("SELECT 1")


def build_gateway_channel(settings: Settings) -> grpc.aio.Channel:
    """Lazy-connecting aio channel to the Go control-plane gateway (AGENT_GRPC_TARGET)."""
    return grpc.aio.insecure_channel(settings.gateway.agent_grpc_target)


def build_gateway_client(channel: grpc.aio.Channel, settings: Settings) -> TaskGateway:
    """SystemTaskGateway client; the timeout derives from the single
    GATEWAY_RPC_TIMEOUT_MS setting (CR-REG-32) and retries from
    GATEWAY_RPC_MAX_RETRIES. Wired into the lifespan with the servicer batch
    (T07); worker/heartbeat consumers land with T08."""
    return SystemTaskGatewayClient(
        channel,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
        max_retries=settings.gateway.gateway_rpc_max_retries,
    )