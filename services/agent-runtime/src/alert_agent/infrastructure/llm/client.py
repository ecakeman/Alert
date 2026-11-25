"""OpenAI-compatible async LLM client shared by diagnosis and RAG (design 5.2,
D7, CR-DIAG-08/09, CR-REG-13~18).

- One lazily created ``httpx.AsyncClient`` behind a double-checked
  ``asyncio.Lock`` (connection reuse; async-only interface, CR-REG-15).
- Structured calls: ``response_format: json_object`` + schema injected into
  the prompt + explicit low temperature (fast 0.1, CR-DIAG-08). Parse pipeline
  (fences -> empty-choices guard -> schema validation) re-asks once with the
  error text; a second failure raises ``MODEL_PARSE_ERROR`` (CR-REG-13).
- Retries: connect/read timeouts and 429/5xx retry with linear backoff up to
  the configured limit; deterministic 4xx never retries (CR-REG-14). API
  failures surface as RetryableError/FatalError so node-level fallback covers
  them (no scripted fallback inside the client, R3).
- Streaming: incremental token passthrough; on any stream failure the client
  falls back to one non-streaming attempt (CR-REG-18).
- All timeouts derive from the single LlmSettings group (CR-REG-32); runtime
  config stays frozen (RAG L14).
"""

from __future__ import annotations

import asyncio
import json
import logging
from collections.abc import AsyncIterator, Callable
from typing import Any

import httpx
from pydantic import BaseModel, TypeAdapter, ValidationError

from alert_agent.config.settings import LlmSettings
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError
from alert_agent.infrastructure.llm.parsing import (
    ModelParseFailure,
    extract_choice_content,
    extract_stream_delta,
    parse_json_object,
)

LOGGER = logging.getLogger(__name__)

_RETRYABLE_STATUS = {429, 500, 502, 503, 504}
_SCHEMA_INSTRUCTION = "\n\n[OUTPUT_SCHEMA] Reply with exactly one JSON object that validates against this JSON Schema (no prose, no fences):\n"

_Validator = Callable[[dict[str, Any]], Any]


class OpenAiCompatibleLlmClient:
    """Implements the ``LlmDecider`` port (T10) and the structured/streaming
    surface consumed by the RAG side (T15~T17, T23)."""

    def __init__(self, settings: LlmSettings) -> None:
        self._base_url = settings.llm_api_base.rstrip("/")
        self._api_key = settings.llm_api_key
        self._model = settings.llm_model
        self._temperature = settings.fast_temperature
        self._connect_timeout = settings.llm_connect_timeout_seconds
        self._read_timeout = settings.llm_read_timeout_seconds
        self._max_retries = settings.llm_max_retries
        self._backoff_seconds = settings.llm_retry_backoff_seconds
        self._client: httpx.AsyncClient | None = None
        self._client_lock = asyncio.Lock()

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def _ensure_client(self) -> httpx.AsyncClient:
        if self._client is None:
            async with self._client_lock:  # double-checked lazy init (RAG L13)
                if self._client is None:
                    timeout = httpx.Timeout(
                        connect=self._connect_timeout,
                        read=self._read_timeout,
                        write=self._read_timeout,
                        pool=self._connect_timeout,
                    )
                    self._client = httpx.AsyncClient(
                        base_url=self._base_url,
                        headers={
                            "Authorization": f"Bearer {self._api_key}",
                            "Content-Type": "application/json",
                        },
                        timeout=timeout,
                    )
        return self._client

    async def decide(
        self,
        *,
        system: str,
        user: str,
        schema_name: str,
        schema: dict[str, Any],
    ) -> dict[str, Any]:
        """``LlmDecider`` port: structured decision JSON (validated by the
        caller's discriminated-union parsing in the application layer)."""
        return await self._complete_json(
            system=system, user=user, schema_name=schema_name, schema=schema
        )

    async def complete_json(
        self, model_cls: type[BaseModel], *, system: str, user: str
    ) -> BaseModel:
        """Structured call returning a validated pydantic instance; schema
        validation failures feed the same single re-ask path (CR-REG-13)."""

        def validator(payload: dict[str, Any]) -> BaseModel:
            try:
                return TypeAdapter(model_cls).validate_python(payload)
            except ValidationError as exc:
                raise ModelParseFailure(f"{model_cls.__name__} schema mismatch: {exc}") from exc

        return await self._complete_json(
            system=system,
            user=user,
            schema_name=model_cls.__name__,
            schema=model_cls.model_json_schema(),
            validator=validator,
        )

    async def _complete_json(
        self,
        *,
        system: str,
        user: str,
        schema_name: str,
        schema: dict[str, Any],
        validator: _Validator | None = None,
    ) -> Any:
        schema_text = json.dumps(schema, ensure_ascii=False)
        messages = [
            {"role": "system", "content": system},
            {"role": "user", "content": f"{user}{_SCHEMA_INSTRUCTION}{schema_text}"},
        ]
        content = await self._chat_json(messages)
        try:
            return self._validate(content, validator)
        except ModelParseFailure as first_failure:
            LOGGER.warning("LLM reply failed parsing (%s); re-asking once", first_failure)
            retry_messages = messages + [
                {"role": "assistant", "content": content},
                {
                    "role": "user",
                    "content": (
                        "Your previous reply failed parsing/validation with this error: "
                        f"{first_failure}. Reply again with exactly one JSON object, "
                        "no prose, no fences."
                    ),
                },
            ]
            retry_content = await self._chat_json(retry_messages)
            try:
                return self._validate(retry_content, validator)
            except ModelParseFailure as exc:
                raise FatalError(
                    ErrorCode.MODEL_PARSE_ERROR,
                    f"structured reply failed after one re-ask: {exc}",
                ) from exc

    @staticmethod
    def _validate(content: str, validator: _Validator | None) -> Any:
        payload = parse_json_object(content)
        if validator is None:
            return payload
        return validator(payload)

    async def _chat_json(self, messages: list[dict[str, Any]]) -> str:
        payload = {
            "model": self._model,
            "messages": messages,
            "temperature": self._temperature,
            "response_format": {"type": "json_object"},
        }
        return extract_choice_content(await self._post_chat(payload))

    async def _complete_text(self, system: str, user: str) -> str:
        """Non-streaming plain-text completion (streaming fallback, CR-REG-18)."""
        payload = {
            "model": self._model,
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            "temperature": self._temperature,
        }
        return extract_choice_content(await self._post_chat(payload))

    async def stream(self, system: str, user: str) -> AsyncIterator[str]:
        """Incremental token passthrough; any stream failure falls back to one
        non-streaming attempt whose text is yielded at once (CR-REG-18)."""
        try:
            async for token in self._stream_once(system, user):
                yield token
            return
        except (httpx.HTTPError, RetryableError, FatalError) as stream_failure:
            LOGGER.warning(
                "LLM stream failed (%s); retrying once without streaming",
                type(stream_failure).__name__,
            )
        content = await self._complete_text(system, user)
        if content:
            yield content

    async def _stream_once(self, system: str, user: str) -> AsyncIterator[str]:
        client = await self._ensure_client()
        payload = {
            "model": self._model,
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            "temperature": self._temperature,
            "stream": True,
        }
        async with client.stream("POST", "/chat/completions", json=payload) as response:
            if response.status_code >= 400:
                await response.aread()
                self._raise_for_status(response.status_code, retried=0)
            async for line in response.aiter_lines():
                if not line.startswith("data:"):
                    continue
                data = line[len("data:") :].strip()
                if not data or data == "[DONE]":
                    if data == "[DONE]":
                        break
                    continue
                try:
                    chunk = json.loads(data)
                except ValueError:
                    continue  # malformed keep-alive/comment chunk: skip, never crash
                token = extract_stream_delta(chunk)
                if token:
                    yield token

    async def _post_chat(self, payload: dict[str, Any]) -> dict[str, Any]:
        client = await self._ensure_client()
        retried = 0
        while True:
            try:
                response = await client.post("/chat/completions", json=payload)
            except httpx.ConnectTimeout as exc:
                raise RetryableError(
                    ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                    f"LLM connect timeout after {self._connect_timeout}s: {exc}",
                ) from exc
            except httpx.ReadTimeout as exc:
                raise RetryableError(
                    ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                    f"LLM read timeout after {self._read_timeout}s: {exc}",
                ) from exc
            except httpx.HTTPError as exc:
                raise RetryableError(
                    ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                    f"LLM transport failure: {type(exc).__name__}",
                ) from exc
            if response.status_code in _RETRYABLE_STATUS and retried < self._max_retries:
                retried += 1
                delay = self._backoff_seconds * retried
                LOGGER.warning(
                    "LLM %d; retry %d/%d in %.1fs",
                    response.status_code,
                    retried,
                    self._max_retries,
                    delay,
                )
                await asyncio.sleep(delay)
                continue
            if response.status_code >= 400:
                self._raise_for_status(response.status_code, retried)
            try:
                return response.json()
            except ValueError as exc:
                raise ModelParseFailure("chat response body is not JSON") from exc

    def _raise_for_status(self, status_code: int, retried: int) -> None:
        if status_code in _RETRYABLE_STATUS:
            raise RetryableError(
                ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
                f"LLM {status_code} persisted after {retried} retries",
            )
        raise FatalError(
            ErrorCode.MODEL_ENDPOINT_UNAVAILABLE,
            f"LLM deterministic rejection {status_code}; not retried",
        )