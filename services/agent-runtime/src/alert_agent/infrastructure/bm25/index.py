"""In-process BM25 index (T16, design 5.1, CR-RAG-08, CR-REG-24, D9).

Single-instance contract (D9): one index per process, assembled by wiring;
it is NOT a distributed store. The asyncio lock protects only the snapshot
swap and the lazy warm-up; tokenization and scoring run outside the lock
(CR-REG-24). Updates mark the snapshot dirty and the rebuild happens once
before the next search -- never a full rebuild per ingested chunk (CR-RAG-08).
Parent chunks are excluded explicitly (``is_parent`` false, CR-REG-24); the
lazy warm-up loads READY documents' chunks from Postgres once. An empty
index is a real, distinguishable state (leg_status carries it, CR-RAG-06).
"""

from __future__ import annotations

import asyncio
import logging

from rank_bm25 import BM25Okapi

from alert_agent.domain.retrieval.chunk import RetrievedChunk

LOGGER = logging.getLogger(__name__)


def tokenize(text: str) -> list[str]:
    """jieba tokenization for BM25 (runs outside the snapshot lock)."""
    return [token.strip() for token in jieba.cut_for_search(text) if token.strip()]


import jieba


class _Snapshot:
    """Immutable index snapshot; swapping it is the only locked operation."""

    __slots__ = ("bm25", "chunks", "tokenized")

    def __init__(
        self,
        chunks: tuple[RetrievedChunk, ...],
        tokenized: list[list[str]],
        bm25: BM25Okapi | None,
    ) -> None:
        self.chunks = chunks
        self.tokenized = tokenized
        self.bm25 = bm25


class InProcessBm25Index:
    """``Bm25Index`` port implementation (T16); single instance per process."""

    def __init__(self, pool) -> None:
        self._pool = pool
        self._lock = asyncio.Lock()
        self._snapshot = _Snapshot((), [], None)
        self._dirty = False
        self._warmed = False

    async def search_bm25(
        self, query: str, *, doc_ids: tuple[str, ...] | None = None, top_k: int = 8
    ) -> list[RetrievedChunk]:
        await self._ensure_ready()
        tokens = tokenize(query)
        snapshot = self._snapshot  # snapshot read: scoring runs lock-free
        if snapshot.bm25 is None or not tokens:
            return []
        scores = snapshot.bm25.get_scores(tokens)
        order = sorted(range(len(scores)), key=lambda i: -scores[i])
        allowed = set(doc_ids) if doc_ids else None
        results: list[RetrievedChunk] = []
        for index in order:
            if scores[index] <= 0:
                break
            chunk = snapshot.chunks[index]
            if allowed is not None and chunk.doc_id not in allowed:
                continue
            results.append(chunk.model_copy(update={"score": float(scores[index])}))
            if len(results) >= top_k:
                break
        return results

    async def update(self, chunks: list[RetrievedChunk]) -> None:
        """Incremental update: append non-parent chunks and mark dirty; the
        rebuild happens once before the next search (CR-RAG-08)."""
        fresh = [chunk for chunk in chunks if chunk.parent_id is None]
        if not fresh:
            return
        async with self._lock:
            merged = self._snapshot.chunks + tuple(fresh)
            self._snapshot = _Snapshot(merged, [], None)
            self._dirty = True

    async def _ensure_ready(self) -> None:
        if self._warmed and not self._dirty:
            return
        async with self._lock:
            if self._warmed and not self._dirty:
                return
            chunks = list(self._snapshot.chunks)
            if not self._warmed:
                if self._pool is not None:
                    chunks = await self._load_chunks()
                self._warmed = True
            tokenized = [tokenize(chunk.content) for chunk in chunks]
            bm25 = BM25Okapi(tokenized) if tokenized and any(tokenized) else None
            self._snapshot = _Snapshot(tuple(chunks), tokenized, bm25)
            self._dirty = False

    async def _load_chunks(self) -> list[RetrievedChunk]:
        """Warm-up: READY documents' non-parent chunks (single load)."""
        query = """
        SELECT c.chunk_id, c.doc_id, c.parent_id, c.content, c.section_path, c.page
        FROM cognitive.retrieval_chunks c
        JOIN cognitive.retrieval_documents d ON d.doc_id = c.doc_id
        WHERE d.status = 'READY' AND c.is_parent = false
        """
        from psycopg.rows import class_row

        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(_ChunkRow)) as cursor,
        ):
            await cursor.execute(query)
            rows = await cursor.fetchall()
        LOGGER.info("bm25 warm-up loaded %d chunks", len(rows))
        return [
            RetrievedChunk(
                chunk_id=row.chunk_id,
                doc_id=row.doc_id,
                parent_id=row.parent_id,
                content=row.content,
                section_path=row.section_path,
                page=row.page,
                score=0.0,
                leg="bm25",
            )
            for row in rows
        ]


class _ChunkRow:
    """Warm-up projection row (infrastructure only)."""

    chunk_id: str
    doc_id: str
    parent_id: str | None
    content: str
    section_path: str | None
    page: int | None