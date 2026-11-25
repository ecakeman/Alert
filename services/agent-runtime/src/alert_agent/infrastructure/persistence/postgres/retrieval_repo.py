"""Vector-leg store and parent fetcher on cognitive.retrieval_* (T16,
CR-RAG-04, CR-REG-24).

The vector leg ranks non-parent chunks with pgvector cosine distance
(dimensionless vector column, no HNSW index -- sequential scan is the accepted
trade-off, migration 0003); scores are converted to cosine similarity. The
parent fetcher returns parent-chunk bodies for the single expansion
implementation in ``application/retrieval/searcher.py`` (CR-RAG-18).
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
    """pgvector text literal for a dimensionless vector column (D7)."""
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
    """``VectorStore`` port implementation (T16)."""

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
    """Parent-chunk bodies for the single expansion implementation."""

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