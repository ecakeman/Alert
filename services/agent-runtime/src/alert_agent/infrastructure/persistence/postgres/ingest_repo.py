"""cognitive.ingest_jobs 上的摄取任务存储（T19，CR-ING-02/03/06）。

持久化的任务行是权威；doc-id 幂等通过 INSERT ... ON CONFLICT DO NOTHING
实现（CR-ING-03）；状态迁移写入不带 generation 条件的任务行（status CHECK
约束在迁移中定义，CR-REG-37）。
"""

from __future__ import annotations

from psycopg.rows import class_row
from psycopg_pool import AsyncConnectionPool

from alert_agent.domain.ingestion.job import IngestJob, IngestStatus

_CREATE = """
INSERT INTO cognitive.ingest_jobs
    (doc_id, status, callback_url, file_path)
VALUES (%s, 'QUEUED', %s, %s)
ON CONFLICT (doc_id) DO NOTHING
RETURNING doc_id
"""

_SELECT = """
SELECT doc_id, status, chunk_count, error_msg, callback_url, attempt, file_path
FROM cognitive.ingest_jobs
WHERE doc_id = %s
"""

_MARK = """
UPDATE cognitive.ingest_jobs
SET status = %s,
    chunk_count = %s,
    error_msg = %s,
    attempt = attempt + 1,
    updated_at = now()
WHERE doc_id = %s
RETURNING doc_id
"""

_LIST_QUEUED = """
SELECT doc_id, status, chunk_count, error_msg, callback_url, attempt, file_path
FROM cognitive.ingest_jobs
WHERE status = 'QUEUED'
ORDER BY created_at
LIMIT %s
"""


class _JobRow:
    doc_id: str
    status: str
    chunk_count: int | None
    error_msg: str
    callback_url: str
    attempt: int
    file_path: str


class PostgresIngestJobStore:
    """``IngestJobStore`` 端口实现（T19）。"""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    @staticmethod
    def _to_job(row: _JobRow) -> IngestJob:
        return IngestJob(
            doc_id=row.doc_id,
            status=row.status,  # type: ignore[arg-type]
            chunk_count=row.chunk_count,
            error_msg=row.error_msg,
            callback_url=row.callback_url,
            attempt=row.attempt,
            file_path=row.file_path,
        )

    async def create(self, job: IngestJob) -> bool:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _CREATE, (job.doc_id, job.callback_url, job.file_path)
            )
            row = await cursor.fetchone()
        return row is not None

    async def get(self, doc_id: str) -> IngestJob | None:
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_JobRow)) as cursor,
        ):
            await cursor.execute(_SELECT, (doc_id,))
            row = await cursor.fetchone()
        return self._to_job(row) if row else None

    async def mark(
        self,
        doc_id: str,
        status: IngestStatus,
        *,
        chunk_count: int | None = None,
        error_msg: str = "",
    ) -> bool:
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_JobRow)) as cursor,
        ):
            await cursor.execute(
                _MARK, (status, chunk_count, error_msg[:500], doc_id)
            )
            row = await cursor.fetchone()
        return row is not None

    async def list_queued(self, limit: int = 10) -> list[IngestJob]:
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_JobRow)) as cursor,
        ):
            await cursor.execute(_LIST_QUEUED, (limit,))
            rows = await cursor.fetchall()
        return [self._to_job(row) for row in rows]