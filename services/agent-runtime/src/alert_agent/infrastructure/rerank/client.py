"""Rerank client (T16, design 5.1, CR-RAG-07, CR-REG-22/23).

Shared ``httpx.AsyncClient`` with single-source timeouts (CR-REG-23); failures
surface to the caller, which keeps the fusion order and marks the rerank leg
degraded (CR-REG-22) -- the client never silently truncates results.
"""

from __future__ import annotations

import asyncio
import logging

import httpx

from alert_agent.config.settings import LlmSettings
from alert_agent.domain.retrieval.chunk import RetrievedChunk
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

LOGGER = logging.getLogger(__name__)


class OpenAiCompatibleReranker:
    """``Reranker`` port implementation (T16)."""

    def __init__(self, settings: LlmSettings) -> None:
        self._base_url = settings.rerank_api_base.rstrip("/")
        self._api_key = settings.rerank_api_key
        self._model = settings.rerank_model
        self._timeout = httpx.Timeout(
            connect=settings.llm_connect_timeout_seconds,
            read=settings.llm_read_timeout_seconds,
            write=settings.llm_read_timeout_seconds,
            pool=settings.llm_connect_timeout_seconds,
        )
        self._client: httpx.AsyncClient | None = None
        self._client_lock = asyncio.Lock()

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def _ensure_client(self) -> httpx.AsyncClient:
        if self._client is None:
            async with self._client_lock:
                if self._client is None:
                    self._client = httpx.AsyncClient(
                        base_url=self._base_url,
                        headers={
                            "Authorization": f"Bearer {self._api_key}",
                            "Content-Type": "application/json",
                        },
                        timeout=self._timeout,
                    )
        return self._client

    async def rerank(
        self, query: str, chunks: list[RetrievedChunk], *, top_k: int = 5
    ) -> list[RetrievedChunk]:
        if not chunks:
            return []
        client = await self._ensure_client()
        documents = [chunk.content for chunk in chunks]
        try:
            response = await client.post(
                "/rerank",
                json={"model": self._model, "query": query, "documents": documents},
            )
        except httpx.HTTPError as exc:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"rerank transport failure: {type(exc).__name__}",
            ) from exc
        if response.status_code >= 400:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"rerank endpoint returned {response.status_code}",
            )
        results = response.json().get("results")
        if not isinstance(results, list):
            raise FatalError(ErrorCode.INPUT_INVALID, "rerank response shape mismatch")
        ordered: list[RetrievedChunk] = []
        for entry in results[:top_k]:
            index = entry.get("index")
            if not isinstance(index, int) or index < 0 or index >= len(chunks):
                continue  # out-of-range indexes are dropped, never crash
            score = entry.get("relevance_score")
            chunk = chunks[index]
            ordered.append(
                chunk.model_copy(
                    update={"score": float(score) if score is not None else chunk.score}
                )
            )
        return ordered