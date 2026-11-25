"""SSE 帧编码器（T23，design 3.2.3，CR-RAG-17）。

五类事件，数据字段与契约完全一致的 camelCase；帧格式为
``event: <name>\\ndata: <json>\\n\\n``。载荷模型来自 domain.retrieval.sse，
别名由其携带，因此这里不出现手写的 camelCase 字符串。
"""

from __future__ import annotations

import json

from alert_agent.domain.retrieval.sse import (
    CitationsEventData,
    DoneEventData,
    ErrorEventData,
    TokenEventData,
    TraceEventData,
)

EVENT_TRACE = "trace"
EVENT_TOKEN = "token"
EVENT_CITATIONS = "citations"
EVENT_DONE = "done"
EVENT_ERROR = "error"


def encode_event(event: str, data: dict) -> str:
    """编码单个 SSE 帧；不可序列化的值安全降级为 str。"""
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False, default=str)}\n\n"


def trace_frame(node: str, status: str, detail: str, ts_ms: float) -> str:
    return encode_event(
        EVENT_TRACE,
        TraceEventData(node=node, status=status, detail=detail, ts_ms=ts_ms).model_dump(
            by_alias=True
        ),
    )


def token_frame(text: str) -> str:
    return encode_event(EVENT_TOKEN, TokenEventData(text=text).model_dump(by_alias=True))


def citations_frame(items) -> str:
    return encode_event(
        EVENT_CITATIONS,
        CitationsEventData(items=tuple(items)).model_dump(by_alias=True),
    )


def done_frame(latency_ms: float) -> str:
    return encode_event(
        EVENT_DONE, DoneEventData(latency_ms=latency_ms).model_dump(by_alias=True)
    )


def error_frame(code: str, message: str) -> str:
    return encode_event(
        EVENT_ERROR, ErrorEventData(code=code, message=message).model_dump(by_alias=True)
    )