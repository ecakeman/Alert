"""Ingestion HTTP callback client (T20, CR-ING-05).

Three-state callbacks carry the internal token; failures get bounded retries
with backoff and are surfaced as a False return -- the persisted job table
stays the reconciliation authority, so a permanently failed callback is never
silent at the status-query layer (CR-ING-06).
"""

from __future__ import annotations

import asyncio
import logging

import httpx

LOGGER = logging.getLogger(__name__)


class HttpCallbackSender:
    """``CallbackSender`` port implementation (T20)."""

    def __init__(
        self,
        *,
        internal_token: str,
        timeout_seconds: float = 10.0,
        max_retries: int = 3,
        backoff_seconds: float = 1.0,
    ) -> None:
        self._token = internal_token
        self._timeout = httpx.Timeout(
            connect=timeout_seconds / 2.0,
            read=timeout_seconds,
            write=timeout_seconds,
            pool=timeout_seconds / 2.0,
        )
        self._max_retries = max_retries
        self._backoff = backoff_seconds

    async def send(
        self,
        callback_url: str,
        *,
        status: str,
        chunk_count: int | None = None,
        error_msg: str = "",
    ) -> bool:
        payload: dict = {"status": status, "errorMsg": error_msg}
        if chunk_count is not None:
            payload["chunkCount"] = chunk_count
        headers = {
            "X-Internal-Token": self._token,
            "Content-Type": "application/json",
        }
        for attempt in range(self._max_retries + 1):
            try:
                async with httpx.AsyncClient(timeout=self._timeout) as client:
                    response = await client.post(callback_url, json=payload, headers=headers)
                if response.status_code < 400:
                    return True
                LOGGER.warning(
                    "ingest callback %s got %d (attempt %d)", callback_url, response.status_code, attempt
                )
            except httpx.HTTPError as exc:
                LOGGER.warning(
                    "ingest callback %s transport failure: %s (attempt %d)",
                    callback_url,
                    type(exc).__name__,
                    attempt,
                )
            if attempt < self._max_retries:
                await asyncio.sleep(self._backoff * (attempt + 1))
        LOGGER.error(
            "ingest callback %s failed after %d attempts; job table remains the authority",
            callback_url,
            self._max_retries + 1,
        )
        return False