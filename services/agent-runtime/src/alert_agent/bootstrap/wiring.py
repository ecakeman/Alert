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

from psycopg_pool import AsyncConnectionPool

from alert_agent.application.task.ports import ReceiptStore
from alert_agent.application.task.submit import SubmitTask
from alert_agent.infrastructure.persistence.postgres.pool import build_pool
from alert_agent.infrastructure.persistence.postgres.receipt_repo import PostgresReceiptStore

__all__ = [
    "SystemClock",
    "UuidIdGen",
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