"""cognitive.retrieval_* 上的向量腿存储与父块获取器（T16，CR-RAG-04，
CR-REG-24）。

向量腿用 pgvector 余弦距离对非父块排序（无维度 vector 列，无 HNSW 索引——
顺序扫描是接受的取舍，迁移 0003）；分数转换为余弦相似度。父块获取器返回
父块正文，供 ``application/retrieval/searcher.py`` 中的唯一扩展实现使用
（CR-RAG-18）。
"""

from __future__ import annotations

from psycopg.rows import class_row
from psycopg_pool import AsyncConnectionPool

from alert_agent.domain.retrieval.chunk import RetrievedChunk

_VECTOR_SEARCH = """
SELECT chunk_id, doc_id, parent_id, content, section_path, page,
       1 - (embedding <=> (%s)::vector) AS similarity
FROM cognitive.retrieval_chunks
WHERE is_parent = false AND embedding IS NOT NULL
ORDER BY embedding <=> (%s)::vector
LIMIT %s
"""

_FETCH_PARENTS = """
SELECT chunk_id, doc_id, parent_id, content, section_path, page
FROM cognitive.retrieval_chunks
WHERE is_parent = true AND chunk_id = ANY(%s)
"""


def _vector_literal(embedding: list[float]) -> str:
    """面向无维度 vector 列的 pgvector 文本字面量（D7）。"""
    return "[" + ",".join(f"{component:.6f}" for component in embedding) + "]"


class _ChunkRow:
    chunk_id: str
    doc_id: str
    parent_id: str | None
    content: str
    section_path: str | None
    page: int | None
    similarity: float | None = None


class PostgresVectorStore:
    """``VectorStore`` 端口实现（T16）。"""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def search_vector(
        self,
        embedding: list[float],
        *,
        doc_ids: tuple[str, ...] | None = None,
        top_k: int = 8,
    ) -> list[RetrievedChunk]:
        literal = _vector_literal(embedding)
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_ChunkRow)) as cursor,
        ):
            await cursor.execute(_VECTOR_SEARCH, (literal, literal, top_k))
            rows = await cursor.fetchall()
        results: list[RetrievedChunk] = []
        for row in rows:
            if doc_ids is not None and row.doc_id not in doc_ids:
                continue
            results.append(
                RetrievedChunk(
                    chunk_id=row.chunk_id,
                    doc_id=row.doc_id,
                    parent_id=row.parent_id,
                    content=row.content,
                    section_path=row.section_path,
                    page=row.page,
                    score=float(row.similarity or 0.0),
                    leg="vector",
                )
            )
        return results


class PostgresParentFetcher:
    """为唯一的扩展实现提供父块正文。"""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def fetch_parents(self, parent_ids: tuple[str, ...]) -> list[RetrievedChunk]:
        if not parent_ids:
            return []
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_ChunkRow)) as cursor,
        ):
            await cursor.execute(_FETCH_PARENTS, (list(parent_ids),))
            rows = await cursor.fetchall()
        return [
            RetrievedChunk(
                chunk_id=row.chunk_id,
                doc_id=row.doc_id,
                parent_id=row.parent_id,
                content=row.content,
                section_path=row.section_path,
                page=row.page,
                score=0.0,
                leg="parent",
            )
            for row in rows
        ]