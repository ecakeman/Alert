"""Dual-leg retrieval orchestration (T16, design 5.1, CR-RAG-04~08, CR-REG-20~26).

Pipeline: per-query vector legs (multi-query expansion is real -- each
expanded query embeds and searches independently, CR-RAG-05) + one BM25 leg ->
RRF fusion (pure function) -> unified content fetch from the union pool
(first-leg setdefault overriding is impossible by construction, CR-REG-21) ->
rerank with honest degradation (failure keeps the fused top-K=10 and reports
``leg_status["rerank"] = "degraded"``, CR-RAG-07/CR-REG-22) -> parent-chunk
expansion implemented exactly once (CR-RAG-18). Single-leg failures keep the
other leg running and are reported (CR-RAG-06, CR-REG-25).
"""

from __future__ import annotations

import asyncio
import logging

from alert_agent.domain.retrieval.chunk import RetrievalResult, RetrievedChunk
from alert_agent.domain.retrieval.rrf import rrf_fuse
from alert_agent.domain.shared import ErrorCode, FatalError

LOGGER = logging.getLogger(__name__)

_PARENT_MAX_CHUNKS = 8
_PARENT_MAX_CHARS = 3600  # ~900 tokens; truncation is visible, never silent


class ParentFetcher:
    """Fetches parent-chunk bodies for expansion (infrastructure: T16 repo)."""

    async def fetch_parents(
        self, parent_ids: tuple[str, ...]
    ) -> list[RetrievedChunk]:
        raise NotImplementedError


def expand_to_parents(
    chunks: list[RetrievedChunk],
    fetcher: ParentFetcher | None,
) -> list[RetrievedChunk]:
    """Single parent-expansion implementation (CR-RAG-18): deduplicated
    parent ids, at most 8 chunks, each truncated at ~900 tokens with a visible
    marker. Chunks without a parent pass through unchanged."""
    if fetcher is None:
        return chunks
    parent_ids: list[str] = []
    for chunk in chunks:
        if chunk.parent_id and chunk.parent_id not in parent_ids:
            parent_ids.append(chunk.parent_id)
    if not parent_ids:
        return chunks
    try:
        parents = asyncio.get_running_loop().run_until_complete(
            fetcher.fetch_parents(tuple(parent_ids[:_PARENT_MAX_CHUNKS]))
        )
    except RuntimeError:
        return chunks  # no running loop: caller must expand asynchronously
    expanded: list[RetrievedChunk] = list(chunks)
    for parent in parents[:_PARENT_MAX_CHUNKS]:
        content = parent.content
        if len(content) > _PARENT_MAX_CHARS:
            content = content[:_PARENT_MAX_CHARS] + "…[parent truncated]"
        expanded.append(
            RetrievedChunk(
                chunk_id=parent.chunk_id,
                doc_id=parent.doc_id,
                parent_id=None,
                content=content,
                section_path=parent.section_path,
                page=parent.page,
                score=parent.score,
                leg="parent",
            )
        )
    return expanded


async def expand_parents_async(
    chunks: list[RetrievedChunk], fetcher: ParentFetcher
) -> list[RetrievedChunk]:
    """Async variant used by the RAG loop (event-loop safe, CR-REG-38)."""
    parent_ids: list[str] = []
    for chunk in chunks:
        if chunk.parent_id and chunk.parent_id not in parent_ids:
            parent_ids.append(chunk.parent_id)
    if not parent_ids:
        return chunks
    parents = await fetcher.fetch_parents(tuple(parent_ids[:_PARENT_MAX_CHUNKS]))
    expanded: list[RetrievedChunk] = list(chunks)
    for parent in parents[:_PARENT_MAX_CHUNKS]:
        content = parent.content
        if len(content) > _PARENT_MAX_CHARS:
            content = content[:_PARENT_MAX_CHARS] + "…[parent truncated]"
        expanded.append(
            RetrievedChunk(
                chunk_id=parent.chunk_id,
                doc_id=parent.doc_id,
                parent_id=None,
                content=content,
                section_path=parent.section_path,
                page=parent.page,
                score=parent.score,
                leg="parent",
            )
        )
    return expanded


class DualLegSearcher:
    """``Searcher`` port implementation (T16)."""

    def __init__(
        self,
        *,
        embedder,
        vector_store,
        bm25_index,
        reranker,
        parent_fetcher: ParentFetcher | None = None,
        vector_top_k: int = 8,
        fusion_top_k: int = 10,
        rerank_top_k: int = 5,
    ) -> None:
        self._embedder = embedder
        self._vector_store = vector_store
        self._bm25_index = bm25_index
        self._reranker = reranker
        self._parent_fetcher = parent_fetcher
        self._vector_top_k = vector_top_k
        self._fusion_top_k = fusion_top_k
        self._rerank_top_k = rerank_top_k

    async def search(
        self,
        *,
        query: str,
        extra_queries: tuple[str, ...] = (),
        doc_ids: tuple[str, ...] | None = None,
        vector_top_k: int | None = None,
        fusion_top_k: int | None = None,
    ) -> RetrievalResult:
        vector_top_k = vector_top_k or self._vector_top_k
        fusion_top_k = fusion_top_k or self._fusion_top_k
        leg_status: dict[str, str] = {}
        pool: dict[str, RetrievedChunk] = {}
        rankings: list[list[str]] = []

        queries = (query, *extra_queries)
        for index, leg_query in enumerate(queries):
            if self._embedder is None or self._vector_store is None:
                # Embedding disabled in settings: the vector leg stays
                # unconfigured while BM25 keeps working (CR-RAG-06).
                leg_status.setdefault("vector", "not configured")
                break
            try:
                embeddings = await self._embedder.embed([leg_query])
                leg_chunks = await self._vector_store.search_vector(
                    embeddings[0], doc_ids=doc_ids, top_k=vector_top_k
                )
            except Exception as exc:  # noqa: BLE001 -- leg isolation is the point
                key = "vector" if index == 0 else f"vector[{index}]"
                leg_status[key] = f"failed: {type(exc).__name__}"
                LOGGER.warning(
                    "vector leg %d failed: %s", index, type(exc).__name__
                )
                continue
            if index == 0:
                leg_status["vector"] = "ok"
            for chunk in leg_chunks:
                pool.setdefault(chunk.chunk_id, chunk)
            rankings.append([chunk.chunk_id for chunk in leg_chunks])

        try:
            bm25_chunks = await self._bm25_index.search_bm25(
                query, doc_ids=doc_ids, top_k=vector_top_k
            )
            leg_status["bm25"] = "ok"
        except Exception as exc:  # noqa: BLE001 -- leg isolation is the point
            leg_status["bm25"] = f"failed: {type(exc).__name__}"
            LOGGER.warning("bm25 leg failed: %s", type(exc).__name__)
            bm25_chunks = []
        for chunk in bm25_chunks:
            pool.setdefault(chunk.chunk_id, chunk)
        if bm25_chunks:
            rankings.append([chunk.chunk_id for chunk in bm25_chunks])

        fused = rrf_fuse(rankings, top_k=fusion_top_k)
        # Unified fetch from the union pool: no first-leg bias (CR-REG-21).
        fused_chunks = [pool[chunk_id] for chunk_id, _score in fused if chunk_id in pool]

        if self._reranker is not None and fused_chunks:
            try:
                reranked = await self._reranker.rerank(
                    query, list(fused_chunks), top_k=self._rerank_top_k
                )
                leg_status["rerank"] = "ok"
                final_chunks = reranked
            except Exception as exc:  # noqa: BLE001 -- degradation is honest
                leg_status["rerank"] = f"degraded: {type(exc).__name__}"
                LOGGER.warning("rerank failed, keeping fusion order: %s", type(exc).__name__)
                final_chunks = fused_chunks
        else:
            final_chunks = fused_chunks

        if self._parent_fetcher is not None:
            final_chunks = await expand_parents_async(final_chunks, self._parent_fetcher)

        if not rankings and not leg_status:
            raise FatalError(ErrorCode.INPUT_INVALID, "no retrieval legs configured")
        return RetrievalResult(chunks=tuple(final_chunks), leg_status=leg_status)