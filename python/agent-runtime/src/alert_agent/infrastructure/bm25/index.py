"""进程内 BM25 索引（T16，design 5.1，CR-RAG-08，CR-REG-24，D9）。

单实例契约（D9）：每个进程一个索引，由装配层组装；它不是分布式存储。
asyncio 锁只保护快照交换与惰性预热；分词与打分在锁外运行（CR-REG-24）。
更新将快照标记为脏，重建在下一次搜索前仅执行一次——绝不按每个摄取的
块全量重建（CR-RAG-08）。父块被显式排除（``is_parent`` 为假，CR-REG-24）；
惰性预热一次性从 Postgres 加载 READY 文档的块。空索引是真实且可区分的
状态（由 leg_status 承载，CR-RAG-06）。
"""

from __future__ import annotations

import asyncio
import logging

from rank_bm25 import BM25Okapi

from alert_agent.domain.retrieval.chunk import RetrievedChunk

LOGGER = logging.getLogger(__name__)


def tokenize(text: str) -> list[str]:
    """BM25 的 jieba 分词（在快照锁之外运行）。"""
    return [token.strip() for token in jieba.cut_for_search(text) if token.strip()]


import jieba


class _Snapshot:
    """不可变的索引快照；交换快照是唯一加锁的操作。"""

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
    """``Bm25Index`` 端口实现（T16）；每个进程单实例。"""

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
        snapshot = self._snapshot  # 快照读取：打分在无锁状态下运行
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
        """增量更新：追加非父块并标记脏状态；重建在下一次搜索前
        执行一次（CR-RAG-08）。"""
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
        """预热：READY 文档的非父块（仅加载一次）。"""
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
    """预热投影行（仅基础设施层使用）。"""

    chunk_id: str
    doc_id: str
    parent_id: str | None
    content: str
    section_path: str | None
    page: int | None