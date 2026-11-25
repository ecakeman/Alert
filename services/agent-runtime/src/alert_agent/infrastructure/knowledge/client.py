"""A2A knowledge-invocation client (T22, design 4.5.2, CR-KNOW-01~09).

Implements the ``KnowledgeInvoker`` port over A2A HTTP: ``InvocationV1`` is
validated once and serialized with ``model_dump(by_alias=True)`` (no
hand-written camelCase strings, CR-REG-30); ``knownFacts`` is JSON-serialized
with ``ensure_ascii=False`` (CR-KNOW-02); constraints express topK and
requireCitations explicitly (CR-KNOW-03). ``status != COMPLETED`` raises a
diagnosable error carrying error_code and safe_message (CR-KNOW-08); results
validate as ``list[KnowledgeHit]`` before entering diagnosis state
(CR-KNOW-07). Transport failures get bounded retries; deterministic business
rejections never retry (CR-KNOW-08). A W3C ``traceparent`` is forwarded only
when the caller supplies one -- never fabricated (CR-KNOW-06).
"""

from __future__ import annotations

import asyncio
import json
import logging

import httpx

from alert_agent.domain.knowledge.models import InvocationV1, KnowledgeHit
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

LOGGER = logging.getLogger(__name__)


class A2aKnowledgeClient:
    """``KnowledgeInvoker`` port implementation (T22)."""

    def __init__(
        self,
        *,
        endpoint: str,
        token: str,
        timeout_seconds: float,
        max_retries: int = 2,
        backoff_seconds: float = 0.5,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._endpoint = endpoint.rstrip("/")
        self._token = token
        self._timeout = httpx.Timeout(
            connect=timeout_seconds / 2.0,
            read=timeout_seconds,
            write=timeout_seconds,
            pool=timeout_seconds / 2.0,
        )
        self._max_retries = max_retries
        self._backoff = backoff_seconds
        self._transport = transport  # test seam; None uses the default stack

    async def invoke(
        self, invocation: InvocationV1, *, traceparent: str | None = None
    ) -> list[KnowledgeHit]:
        # CR-KNOW-01/02: the validated model serializes once, by alias;
        # knownFacts is JSON text with ensure_ascii=False, never repr().
        payload = invocation.model_dump(by_alias=True)
        payload["context"] = dict(payload.get("context") or {})
        known_facts = payload["context"].get("knownFacts") or []
        payload["context"]["knownFactsText"] = json.dumps(known_facts, ensure_ascii=False)
        headers = {
            "Authorization": f"Bearer {self._token}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        }
        if traceparent:
            headers["traceparent"] = traceparent  # forward-only, never fabricated
        message = {
            "role": "user",
            "parts": [{"kind": "data", "data": payload}],
            "metadata": {"invocationId": invocation.invocation_id, "skill": invocation.skill},
        }
        body = {"message": message}
        last_error: Exception | None = None
        for attempt in range(self._max_retries + 1):
            try:
                async with httpx.AsyncClient(timeout=self._timeout, transport=self._transport) as client:
                    response = await client.post(self._endpoint, json=body, headers=headers)
            except httpx.ConnectTimeout as exc:
                last_error = exc
                if attempt < self._max_retries:
                    await asyncio.sleep(self._backoff * (attempt + 1))
                continue
            except httpx.ReadTimeout as exc:
                last_error = exc
                if attempt < self._max_retries:
                    await asyncio.sleep(self._backoff * (attempt + 1))
                continue
            except httpx.HTTPError as exc:
                last_error = exc
                if attempt < self._max_retries:
                    await asyncio.sleep(self._backoff * (attempt + 1))
                continue
            if response.status_code >= 400:
                # Deterministic business rejection: no retry (CR-KNOW-08).
                raise FatalError(
                    ErrorCode.KNOWLEDGE_INVOCATION_FAILED,
                    f"knowledge agent rejected the invocation ({response.status_code})",
                )
            return self._parse(response.json())
        raise RetryableError(
            ErrorCode.KNOWLEDGE_INVOCATION_FAILED,
            f"knowledge invocation failed after {self._max_retries + 1} attempts:"
            f" {type(last_error).__name__}",
        )

    def _parse(self, body: dict) -> list[KnowledgeHit]:
        status = body.get("status")
        if status != "COMPLETED":
            raise FatalError(
                ErrorCode.KNOWLEDGE_INVOCATION_FAILED,
                f"knowledge status {status}: {body.get('safe_message', '')}"
                f" (error_code={body.get('error_code', '')})",
            )
        result = body.get("result") or {}
        raw_hits = result.get("results") or []
        if not isinstance(raw_hits, list):
            raise FatalError(
                ErrorCode.KNOWLEDGE_SCHEMA_ERROR,
                "knowledge result.results is not a list",
            )
        try:
            return [KnowledgeHit.model_validate(entry) for entry in raw_hits]
        except Exception as exc:
            raise FatalError(
                ErrorCode.KNOWLEDGE_SCHEMA_ERROR,
                f"knowledge hit schema mismatch: {type(exc).__name__}",
            ) from exc