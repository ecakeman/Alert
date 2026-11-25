"""OpenAI-compatible embedding client (T16, design 5.1, CR-RAG-04, D7).

Batch embeddings against ``/embeddings``; one lazily created shared
``httpx.AsyncClient`` (double-checked lock, connection reuse); timeouts derive
from the single LlmSettings group (CR-REG-32). Dimension consistency with the
DB vector columns is validated at startup from ``embed_dim`` (D7).
"""

from __future__ import annotations

import asyncio
import logging

import httpx

from alert_agent.config.settings import LlmSettings
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

LOGGER = logging.getLogger(__name__)


class OpenAiCompatibleEmbedder:
    """``Embedder`` port implementation (T16)."""

    def __init__(self, settings: LlmSettings, batch_size: int = 16) -> None:
        self._base_url = settings.embedding_api_base.rstrip("/")
        self._api_key = settings.embedding_api_key
        self._model = settings.embedding_model
        self._embed_dim = settings.embed_dim
        self._batch_size = batch_size
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

    async def embed(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            return []
        vectors: list[list[float]] = []
        for start in range(0, len(texts), self._batch_size):
            vectors.extend(await self._embed_batch(texts[start : start + self._batch_size]))
        return vectors

    async def _embed_batch(self, texts: list[str]) -> list[list[float]]:
        client = await self._ensure_client()
        try:
            response = await client.post(
                "/embeddings", json={"model": self._model, "input": texts}
            )
        except httpx.ConnectTimeout as exc:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"embedding connect timeout: {exc}",
            ) from exc
        except httpx.ReadTimeout as exc:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"embedding read timeout: {exc}",
            ) from exc
        except httpx.HTTPError as exc:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"embedding transport failure: {type(exc).__name__}",
            ) from exc
        if response.status_code >= 400:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"embedding endpoint returned {response.status_code}",
            )
        payload = response.json()
        data = payload.get("data")
        if not isinstance(data, list) or len(data) != len(texts):
            raise FatalError(
                ErrorCode.INPUT_INVALID, "embedding response shape mismatch"
            )
        vectors: list[list[float]] = []
        for item in sorted(data, key=lambda entry: entry.get("index", 0)):
            vector = item.get("embedding")
            if not isinstance(vector, list) or not vector:
                raise FatalError(ErrorCode.INPUT_INVALID, "embedding vector missing")
            if self._embed_dim and len(vector) != self._embed_dim:
                raise FatalError(
                    ErrorCode.INPUT_INVALID,
                    f"embedding dim {len(vector)} != configured embed_dim {self._embed_dim} (D7)",
                )
            vectors.append(vector)
        return vectors