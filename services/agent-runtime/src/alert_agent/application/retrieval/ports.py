"""RAG retrieval ports (design 3.2/5.1, batches T15~T18).

Ports speak domain models only (CR-LAY-07): dual-leg data sources
(``VectorStore``/``Bm25Index``), embedding and rerank clients, the structured
and streaming LLM surfaces (``LlmJson``/``Streamer``) and the composed
``Searcher`` contract whose result carries ``leg_status`` (CR-RAG-06).
Implementations land in infrastructure (T16); no technology imports here
(CR-LAY-02).
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol

from pydantic import BaseModel

from alert_agent.domain.retrieval.chunk import RetrievalResult, RetrievedChunk


class VectorStore(Protocol):
    """Vector leg over the pgvector-backed retrieval store (T16)."""

    async def search_vector(
        self,
        embedding: list[float],
        *,
        doc_ids: tuple[str, ...] | None = None,
        top_k: int = 8,
    ) -> list[RetrievedChunk]: ...


class Bm25Index(Protocol):
    """In-process BM25 leg; tokenization and scoring happen inside (D9)."""

    async def search_bm25(
        self, query: str, *, doc_ids: tuple[str, ...] | None = None, top_k: int = 8
    ) -> list[RetrievedChunk]: ...


class Embedder(Protocol):
    """OpenAI-compatible batch embeddings; ``embed_dim`` consistency is
    validated at startup (D7)."""

    async def embed(self, texts: list[str]) -> list[list[float]]: ...


class Reranker(Protocol):
    """Rerank client; failures degrade to fusion order with degraded=true
    (CR-RAG-07, CR-REG-22) -- enforced by the caller."""

    async def rerank(
        self, query: str, chunks: list[RetrievedChunk], *, top_k: int = 5
    ) -> list[RetrievedChunk]: ...


class Searcher(Protocol):
    """Composed dual-leg retrieval: vector + BM25, RRF fusion, fetch of chunk
    content and leg_status reporting (implemented by T16 ``searcher.py``)."""

    async def search(
        self,
        *,
        query: str,
        doc_ids: tuple[str, ...] | None = None,
        vector_top_k: int = 8,
        fusion_top_k: int = 10,
    ) -> RetrievalResult: ...


class LlmJson(Protocol):
    """Structured JSON calls returning validated pydantic instances; parse
    failures re-ask once inside the implementation (CR-REG-13)."""

    async def complete_json(
        self, model_cls: type[BaseModel], *, system: str, user: str
    ) -> BaseModel: ...


class Streamer(Protocol):
    """Incremental token stream; streaming failure falls back to one
    non-streaming attempt inside the implementation (CR-REG-18)."""

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