"""A2A 知识调用客户端（T22，design 4.5.2，CR-KNOW-01~09）。

基于 A2A HTTP 实现 ``KnowledgeInvoker`` 端口：``InvocationV1`` 先校验一次，
再以 ``model_dump(by_alias=True)`` 序列化（不手写 camelCase 字符串，
CR-REG-30）；``knownFacts`` 以 ``ensure_ascii=False`` 做 JSON 序列化
（CR-KNOW-02）；constraints 显式表达 topK 与 requireCitations（CR-KNOW-03）。
``status != COMPLETED`` 抛出携带 error_code 与 safe_message 的可诊断错误
（CR-KNOW-08）；结果在进入诊断状态前校验为 ``list[KnowledgeHit]``
（CR-KNOW-07）。传输失败做有界重试；确定性业务拒绝绝不重试（CR-KNOW-08）。
仅当调用方提供 W3C ``traceparent`` 时才转发——绝不伪造（CR-KNOW-06）。
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
    """``KnowledgeInvoker`` 端口实现（T22）。"""

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
        self._transport = transport  # 测试接缝；为 None 时使用默认协议栈

    async def invoke(
        self, invocation: InvocationV1, *, traceparent: str | None = None
    ) -> list[KnowledgeHit]:
        # CR-KNOW-01/02：已校验的模型按别名一次性序列化；
        # knownFacts 是 ensure_ascii=False 的 JSON 文本，绝不用 repr()。
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
            headers["traceparent"] = traceparent  # 仅转发，绝不伪造
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
                # 确定性业务拒绝：不重试（CR-KNOW-08）。
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