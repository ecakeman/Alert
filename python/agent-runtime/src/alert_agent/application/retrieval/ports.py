"""RAG 检索端口（design 3.2/5.1，批次 T15~T18）。

端口只涉及领域模型（CR-LAY-07）：双路数据源
（``VectorStore``/``Bm25Index``）、嵌入与重排客户端、结构化与流式
LLM 接口（``LlmJson``/``Streamer``），以及结果携带 ``leg_status``
的复合 ``Searcher`` 契约（CR-RAG-06）。实现位于基础设施层（T16）；
此处不含任何技术导入（CR-LAY-02）。
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol

from pydantic import BaseModel

from alert_agent.domain.retrieval.chunk import RetrievalResult, RetrievedChunk


class VectorStore(Protocol):
    """基于 pgvector 检索存储的向量路（T16）。"""

    async def search_vector(
        self,
        embedding: list[float],
        *,
        doc_ids: tuple[str, ...] | None = None,
        top_k: int = 8,
    ) -> list[RetrievedChunk]: ...


class Bm25Index(Protocol):
    """进程内 BM25 路；分词与打分均在内部完成（D9）。"""

    async def search_bm25(
        self, query: str, *, doc_ids: tuple[str, ...] | None = None, top_k: int = 8
    ) -> list[RetrievedChunk]: ...


class Embedder(Protocol):
    """OpenAI 兼容的批量嵌入；``embed_dim`` 一致性在启动时校验（D7）。"""

    async def embed(self, texts: list[str]) -> list[list[float]]: ...


class Reranker(Protocol):
    """重排客户端；失败时降级为融合顺序并置 degraded=true
    （CR-RAG-07，CR-REG-22）——由调用方强制执行。"""

    async def rerank(
        self, query: str, chunks: list[RetrievedChunk], *, top_k: int = 5
    ) -> list[RetrievedChunk]: ...


class Searcher(Protocol):
    """复合双路检索：向量 + BM25、RRF 融合、取回分块内容并报告
    leg_status（由 T16 ``searcher.py`` 实现）。"""

    async def search(
        self,
        *,
        query: str,
        doc_ids: tuple[str, ...] | None = None,
        vector_top_k: int = 8,
        fusion_top_k: int = 10,
    ) -> RetrievalResult: ...


class LlmJson(Protocol):
    """返回已校验 pydantic 实例的结构化 JSON 调用；解析失败时在实现内部
    追问一次（CR-REG-13）。"""

    async def complete_json(
        self, model_cls: type[BaseModel], *, system: str, user: str
    ) -> BaseModel: ...


class Streamer(Protocol):
    """增量 token 流；流式失败时在实现内部回退为一次非流式调用
    （CR-REG-18）。"""

    def stream(self, *, system: str, user: str) -> AsyncIterator[str]: ...


__all__ = [
    "Bm25Index",
    "Embedder",
    "LlmJson",
    "Reranker",
    "Searcher",
    "Streamer",
    "VectorStore",
]