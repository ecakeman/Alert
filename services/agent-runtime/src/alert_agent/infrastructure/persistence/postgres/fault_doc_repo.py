"""cognitive.fault_documents / fault_doc_chunks 上的故障文档存储
（T14，CR-ARCH-06~08）。

生命周期：``upsert_building``（episode 幂等键，CR-ARCH-06）->
``mark_ready``（索引字段 + 各分块，含由 jieba 分词构建的 'simple' 配置
tsv 全文腿，即迁移文件中的应用层填充点）-> ``mark_failed`` 保留
``index_error`` 供重试。``search`` 实现 ``FaultDocSearcher`` 端口：仅
READY 记录，同服务优先、跨服务兜底，按 ts_rank 排序（CR-ARCH-07/08）。
嵌入填充后向量腿随之加入（T16 Embedder 集成）；没有就绪检索腿的行绝不
出现（CR-ARCH-08）。
"""

from __future__ import annotations

import json
import logging

import jieba
from psycopg.rows import class_row
from psycopg_pool import AsyncConnectionPool

from alert_agent.application.shared.ports import IdGen
from alert_agent.domain.archive.index import FaultDocument
from alert_agent.domain.diagnosis.records import FdHit

LOGGER = logging.getLogger(__name__)

_UPSERT_BUILDING = """
INSERT INTO cognitive.fault_documents
    (fd_id, episode_id, service_id, task_id, generation,
     summary, symptoms, observations, diagnosis, content_hash,
     index_status, index_error)
VALUES (%s, %s, %s, %s, %s, '', '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '',
        'BUILDING', '')
ON CONFLICT (episode_id) DO UPDATE
SET task_id = EXCLUDED.task_id,
    generation = EXCLUDED.generation,
    service_id = EXCLUDED.service_id,
    index_status = 'BUILDING',
    index_error = '',
    updated_at = now()
RETURNING fd_id
"""

_MARK_READY = """
UPDATE cognitive.fault_documents
SET summary = %s,
    symptoms = %s::jsonb,
    observations = %s::jsonb,
    diagnosis = %s::jsonb,
    content_hash = %s,
    index_status = 'READY',
    index_error = '',
    updated_at = now()
WHERE fd_id = %s
RETURNING fd_id
"""

_DELETE_CHUNKS = "DELETE FROM cognitive.fault_doc_chunks WHERE fd_id = %s"

_INSERT_CHUNK = """
INSERT INTO cognitive.fault_doc_chunks (chunk_id, fd_id, chunk_index, content, tsv)
VALUES (%s, %s, %s, %s, to_tsvector('simple', %s))
"""

_MARK_FAILED = """
UPDATE cognitive.fault_documents
SET index_status = 'FAILED',
    index_error = %s,
    updated_at = now()
WHERE fd_id = %s
RETURNING fd_id
"""

_SEARCH = """
SELECT fd.fd_id,
       fd.episode_id,
       fd.service_id,
       fd.summary,
       c.content AS matched,
       ts_rank(c.tsv, query) AS score,
       fd.diagnosis
FROM cognitive.fault_doc_chunks c
JOIN cognitive.fault_documents fd ON fd.fd_id = c.fd_id
CROSS JOIN plainto_tsquery('simple', %s) AS query
WHERE fd.index_status = 'READY'
  AND c.tsv @@ query
ORDER BY (fd.service_id = %s) DESC, score DESC
LIMIT %s
"""

_CHUNK_FIELDS = ("summary", "symptoms", "observations", "diagnosis")


def _tokens(text: str) -> str:
    """面向 'simple' tsv 配置的 jieba 分词文本（D9 填充点）。"""
    return " ".join(token.strip() for token in jieba.cut_for_search(text) if token.strip())


def _json_list(value: object) -> str:
    return json.dumps(value if isinstance(value, list) else [], ensure_ascii=False)


class PostgresFaultDocStore:
    """``FaultDocStore`` + ``FaultDocSearcher`` 实现（T14）。"""

    def __init__(self, pool: AsyncConnectionPool, ids: IdGen) -> None:
        self._pool = pool
        self._ids = ids

    async def upsert_building(
        self,
        *,
        fd_id: str,
        episode_id: str,
        service_id: str,
        task_id: str,
        generation: int,
    ) -> str:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _UPSERT_BUILDING,
                (fd_id, episode_id, service_id, task_id, generation),
            )
            row = await cursor.fetchone()
        return row[0] if row else fd_id

    async def mark_ready(self, fd_id: str, document: FaultDocument) -> bool:
        index = document.index
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _MARK_READY,
                (
                    index.summary,
                    _json_list(index.symptoms),
                    _json_list(index.observations),
                    _json_list(index.diagnosis),
                    document.content_hash,
                    fd_id,
                ),
            )
            row = await cursor.fetchone()
            if row is None:
                return False
            await connection.execute(_DELETE_CHUNKS, (fd_id,))
            for chunk_index, field in enumerate(_CHUNK_FIELDS):
                content = " ".join(getattr(index, field))
                if not content:
                    continue
                await connection.execute(
                    _INSERT_CHUNK,
                    (
                        self._ids.new_id("fdchunk"),
                        fd_id,
                        chunk_index,
                        content,
                        _tokens(content),
                    ),
                )
        return True

    async def mark_failed(self, fd_id: str, error: str) -> bool:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _MARK_FAILED, (error[:500], fd_id)
            )
            row = await cursor.fetchone()
        return row is not None

    async def search(
        self, service_id: str, query: str, *, top_k: int = 5
    ) -> list[FdHit]:
        """CR-ARCH-07/08：仅 READY 记录，同服务优先，按 ts_rank 排序；
        嵌入列填充后向量腿随之加入（T16）。"""
        tokens = _tokens(query)
        if not tokens:
            return []
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_SearchRow)) as cursor,
        ):
            await cursor.execute(_SEARCH, (tokens, service_id, max(1, top_k)))
            rows = await cursor.fetchall()
        return [
            FdHit(
                fd_id=row.fd_id,
                episode_id=row.episode_id,
                service_id=row.service_id,
                summary=row.summary,
                matched=row.matched,
                score=float(row.score),
                diagnosis=row.diagnosis if isinstance(row.diagnosis, list) else [],
            )
            for row in rows
        ]


class _SearchRow:
    """故障文档搜索的投影行（仅基础设施层使用）。"""

    fd_id: str
    episode_id: str
    service_id: str
    summary: str
    matched: str
    score: float
    diagnosis: object