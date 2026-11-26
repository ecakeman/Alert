"""ReceiptStore 端口的 Postgres 实现（design 4.2，CR-TASK-01~05）。

受理是原子的：先执行 INSERT ... ON CONFLICT (task_id, generation) DO NOTHING
（Q1：绝不先查后插）。只有在冲突时才读取已存在的行 FOR SHARE——该读取
等待并发插入提交，且它是分支判定，不是幂等机制。提交的 generation 低于
已记录的 generation 时以 ``STALE_GENERATION`` 拒绝且零写入（6.1.3.2）；
同 generation 的 FAILED 回执被重新受理并保留原始失败记录（CR-TASK-03）；
每次状态更新都携带 generation（以及需要时的 status）条件（CR-REG-41，Q2）。

migration 0006 增列的租约与边界在 accept 时一并写入，使恢复路径可重建
TaskSubmission（P5.9 GAP-1）；老回执六列为 NULL，保持不可续租语义。
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
     status, callback_state, accepted_at, updated_at,
     lease_token, input_version_from, input_version_to, monitor_watermark,
     app_event_watermark, deadline_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (task_id, generation) DO NOTHING
RETURNING task_id
"""

_SELECT_EXISTING_FOR_SHARE = """
SELECT task_id, generation, task_type, episode_id, service_id, thread_id, status,
       lease_token, input_version_from, input_version_to, monitor_watermark,
       app_event_watermark, deadline_at
FROM cognitive.agent_task_receipts
WHERE task_id = %s AND generation = %s
FOR SHARE
"""

_SELECT_MAX_GENERATION = (
    "SELECT max(generation) FROM cognitive.agent_task_receipts WHERE task_id = %s"
)

_REACCEPT_FAILED = """
UPDATE cognitive.agent_task_receipts
SET status = %s, callback_state = %s, updated_at = %s,
    lease_token = %s, input_version_from = %s, input_version_to = %s,
    monitor_watermark = %s, app_event_watermark = %s, deadline_at = %s
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
    task_id, generation, task_type, episode_id, service_id, thread_id, status,
    lease_token, input_version_from, input_version_to, monitor_watermark,
    app_event_watermark, deadline_at
FROM cognitive.agent_task_receipts
WHERE status IN (%s, %s)
ORDER BY task_id, generation DESC
"""

_LIST_UNCONFIRMED_RESULTS = """
SELECT task_id, generation, task_type, episode_id, service_id, thread_id, status,
       lease_token, input_version_from, input_version_to, monitor_watermark,
       app_event_watermark, deadline_at
FROM cognitive.agent_task_receipts
WHERE status = %s AND callback_state = %s AND lease_token IS NOT NULL
"""


class PostgresReceiptStore:
    """cognitive.agent_task_receipts 上的 ReceiptStore 端口实现。"""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def accept(
        self, submission: TaskSubmission, *, thread_id: str, accepted_at: datetime
    ) -> AcceptOutcome:
        identity = submission.identity
        boundary = submission.boundary
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
                    identity.lease_token,
                    boundary.input_version_from,
                    boundary.input_version_to,
                    boundary.monitor_watermark,
                    boundary.app_event_watermark,
                    boundary.deadline_at,
                ),
            )
            if await cursor.fetchone() is not None:
                return AcceptOutcome(accepted=True, duplicate=False)
            existing = await _fetch_existing_row(connection, identity.task_id, identity.generation)
            if existing is None:
                # 正常情况下不可达：除非在 PK 竞争中胜出的并发插入
                # 在 ON CONFLICT 与本次读取之间回滚；此时重试是安全的。
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
                        identity.lease_token,
                        boundary.input_version_from,
                        boundary.input_version_to,
                        boundary.monitor_watermark,
                        boundary.app_event_watermark,
                        boundary.deadline_at,
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
        lease_token=row.lease_token,
        input_version_from=row.input_version_from,
        input_version_to=row.input_version_to,
        monitor_watermark=row.monitor_watermark,
        app_event_watermark=row.app_event_watermark,
        deadline_at=row.deadline_at,
    )