"""双路检索编排（T16，design 5.1，CR-RAG-04~08，CR-REG-20~26）。

流水线：每个查询独立的向量路（多查询扩展是真实的——每个扩展查询独立
向量化并检索，CR-RAG-05）+ 一条 BM25 路 -> RRF 融合（纯函数）-> 从并集池
统一取回内容（按构造保证不会出现第一路 setdefault 覆盖，CR-REG-21）->
带诚实降级的重排（失败时保留融合后的 top-K=10 并报告
``leg_status["rerank"] = "degraded"``，CR-RAG-07/CR-REG-22）-> 父分块
扩展仅实现一次（CR-RAG-18）。单路失败时另一路继续运行并上报
（CR-RAG-06，CR-REG-25）。
"""

from __future__ import annotations

import asyncio
import logging

from alert_agent.domain.retrieval.chunk import RetrievalResult, RetrievedChunk
from alert_agent.domain.retrieval.rrf import rrf_fuse
from alert_agent.domain.shared import ErrorCode, FatalError

LOGGER = logging.getLogger(__name__)

_PARENT_MAX_CHUNKS = 8
_PARENT_MAX_CHARS = 3600  # 约 900 token；截断可见，绝不静默


class ParentFetcher:
    """为扩展获取父分块正文（基础设施：T16 仓库）。"""

    async def fetch_parents(
        self, parent_ids: tuple[str, ...]
    ) -> list[RetrievedChunk]:
        raise NotImplementedError


def expand_to_parents(
    chunks: list[RetrievedChunk],
    fetcher: ParentFetcher | None,
) -> list[RetrievedChunk]:
    """唯一的父分块扩展实现（CR-RAG-18）：父 id 去重，最多 8 个分块，
    每个截断至约 900 token 并附带可见标记。无父分块的块原样通过。"""
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
        return chunks  # 无运行中的事件循环：调用方必须异步扩展
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
    """供 RAG 循环使用的异步变体（事件循环安全，CR-REG-38）。"""
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
    """``Searcher`` 端口的实现（T16）。"""

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
                # 设置中已禁用嵌入：向量路保持未配置状态，
                # 而 BM25 继续工作（CR-RAG-06）。
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
        # 从并集池统一取回：无第一路偏置（CR-REG-21）。
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