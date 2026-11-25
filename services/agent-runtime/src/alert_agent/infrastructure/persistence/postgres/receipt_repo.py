"""Postgres implementation of the ReceiptStore port (design 4.2, CR-TASK-01~05).

Acceptance is atomic: INSERT ... ON CONFLICT (task_id, generation) DO NOTHING
first (Q1: never select-then-insert). Only on conflict does it read the
existing row FOR SHARE -- that read waits for a concurrent insert to commit
and is a branch decision, not the idempotency mechanism. A submission older
than the recorded generation is rejected with ``STALE_GENERATION`` and zero
writes (6.1.3.2); a FAILED receipt of the same generation is re-accepted with
the original failure record kept (CR-TASK-03); every status update carries
the generation (and where required the status) condition (CR-REG-41, Q2).
"""

from __future__ import annotations

from datetime import datetime

from psycopg import AsyncConnection
from psycopg.rows import class_row
from psycopg_pool import AsyncConnectionPool

from alert_agent.application.task.ports import (
    AcceptOutcome,
    TaskSubmission,
    UnfinishedReceipt,
)
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError
from alert_agent.domain.task import CallbackState, ReceiptStatus
from alert_agent.infrastructure.persistence.postgres.rows import ReceiptRow

_INSERT_RECEIPT = """
INSERT INTO cognitive.agent_task_receipts
    (task_id, generation, task_type, episode_id, service_id, thread_id,
     status, callback_state, accepted_at, updated_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (task_id, generation) DO NOTHING
RETURNING task_id
"""

_SELECT_EXISTING_FOR_SHARE = """
SELECT task_id, generation, task_type, episode_id, service_id, thread_id, status
FROM cognitive.agent_task_receipts
WHERE task_id = %s AND generation = %s
FOR SHARE
"""

_SELECT_MAX_GENERATION = (
    "SELECT max(generation) FROM cognitive.agent_task_receipts WHERE task_id = %s"
)

_REACCEPT_FAILED = """
UPDATE cognitive.agent_task_receipts
SET status = %s, callback_state = %s, updated_at = %s
WHERE task_id = %s AND generation = %s AND status = %s
RETURNING task_id
"""

_MARK_RUNNING = """
UPDATE cognitive.agent_task_receipts
SET status = %s, updated_at = %s
WHERE task_id = %s AND generation = %s AND status = %s
"""

_MARK_FINISHED = """
UPDATE cognitive.agent_task_receipts
SET status = %s, callback_state = %s, last_error_code = %s, last_error_message = %s,
    completed_at = %s, updated_at = %s
WHERE task_id = %s AND generation = %s AND status = %s
"""

_MARK_CALLBACK_STATE = """
UPDATE cognitive.agent_task_receipts
SET callback_state = %s, updated_at = %s
WHERE task_id = %s AND generation = %s
"""

_LIST_UNFINISHED = """
SELECT DISTINCT ON (task_id)
    task_id, generation, task_type, episode_id, service_id, thread_id, status
FROM cognitive.agent_task_receipts
WHERE status IN (%s, %s)
ORDER BY task_id, generation DESC
"""

_LIST_UNCONFIRMED_RESULTS = """
SELECT task_id, generation, task_type, episode_id, service_id, thread_id, status
FROM cognitive.agent_task_receipts
WHERE status = %s AND callback_state = %s
"""


class PostgresReceiptStore:
    """ReceiptStore port implementation on cognitive.agent_task_receipts."""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def accept(
        self, submission: TaskSubmission, *, thread_id: str, accepted_at: datetime
    ) -> AcceptOutcome:
        identity = submission.identity
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _INSERT_RECEIPT,
                (
                    identity.task_id,
                    identity.generation,
                    identity.task_type,
                    identity.episode_id,
                    submission.service_id,
                    thread_id,
                    ReceiptStatus.ACCEPTED,
                    CallbackState.NOT_SENT,
                    accepted_at,
                    accepted_at,
                ),
            )
            if await cursor.fetchone() is not None:
                return AcceptOutcome(accepted=True, duplicate=False)
            existing = await _fetch_existing_row(connection, identity.task_id, identity.generation)
            if existing is None:
                # Unreachable unless the concurrent insert that won the PK race
                # aborted between ON CONFLICT and this read; retrying is safe.
                raise RetryableError(
                    ErrorCode.INPUT_INVALID,
                    "concurrent receipt insert rolled back; retry the submission",
                )
            max_cursor = await connection.execute(_SELECT_MAX_GENERATION, (identity.task_id,))
            max_row = await max_cursor.fetchone()
            max_generation = max_row[0] if max_row is not None else None
            if max_generation is not None and identity.generation < max_generation:
                raise FatalError(
                    ErrorCode.STALE_GENERATION,
                    "submission generation"
                    f" {identity.generation} is older than the recorded generation {max_generation}",
                )
            if existing.status == ReceiptStatus.FAILED:
                cursor = await connection.execute(
                    _REACCEPT_FAILED,
                    (
                        ReceiptStatus.ACCEPTED,
                        CallbackState.NOT_SENT,
                        accepted_at,
                        identity.task_id,
                        identity.generation,
                        ReceiptStatus.FAILED,
                    ),
                )
                if await cursor.fetchone() is not None:
                    return AcceptOutcome(accepted=True, duplicate=False)
            return AcceptOutcome(accepted=True, duplicate=True)

    async def mark_running(
        self, task_id: str, generation: int, *, started_at: datetime
    ) -> bool:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _MARK_RUNNING,
                (ReceiptStatus.RUNNING, started_at, task_id, generation, ReceiptStatus.ACCEPTED),
            )
            return cursor.rowcount > 0

    async def mark_finished(
        self,
        task_id: str,
        generation: int,
        *,
        status: ReceiptStatus,
        error_code: str,
        error_message: str,
        callback_state: CallbackState,
        finished_at: datetime,
    ) -> bool:
        if status not in (ReceiptStatus.COMPLETED, ReceiptStatus.FAILED):
            raise FatalError(
                ErrorCode.INPUT_INVALID, "mark_finished only accepts COMPLETED or FAILED"
            )
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _MARK_FINISHED,
                (
                    status,
                    callback_state,
                    error_code,
                    error_message,
                    finished_at,
                    finished_at,
                    task_id,
                    generation,
                    ReceiptStatus.RUNNING,
                ),
            )
            return cursor.rowcount > 0

    async def mark_callback_state(
        self,
        task_id: str,
        generation: int,
        callback_state: CallbackState,
        *,
        updated_at: datetime,
    ) -> bool:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _MARK_CALLBACK_STATE, (callback_state, updated_at, task_id, generation)
            )
            return cursor.rowcount > 0

    async def list_unfinished(self) -> list[UnfinishedReceipt]:
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(ReceiptRow)) as cursor,
        ):
            await cursor.execute(_LIST_UNFINISHED, (ReceiptStatus.ACCEPTED, ReceiptStatus.RUNNING))
            rows = await cursor.fetchall()
        return [_to_unfinished(row) for row in rows]

    async def list_unconfirmed_results(self) -> list[UnfinishedReceipt]:
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(ReceiptRow)) as cursor,
        ):
            await cursor.execute(
                _LIST_UNCONFIRMED_RESULTS, (ReceiptStatus.COMPLETED, CallbackState.NOT_SENT)
            )
            rows = await cursor.fetchall()
        return [_to_unfinished(row) for row in rows]


async def _fetch_existing_row(
    connection: AsyncConnection, task_id: str, generation: int
) -> ReceiptRow | None:
    async with connection.cursor(row_factory=class_row(ReceiptRow)) as cursor:
        await cursor.execute(_SELECT_EXISTING_FOR_SHARE, (task_id, generation))
        return await cursor.fetchone()


def _to_unfinished(row: ReceiptRow) -> UnfinishedReceipt:
    return UnfinishedReceipt(
        task_id=row.task_id,
        generation=row.generation,
        task_type=row.task_type,
        episode_id=row.episode_id,
        service_id=row.service_id,
        thread_id=row.thread_id,
        status=ReceiptStatus(row.status),
    )