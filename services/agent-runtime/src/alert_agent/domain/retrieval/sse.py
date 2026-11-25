"""SSE 事件协议，共五类事件（design 3.2.3，CR-RAG-17，7.4.7）。

线上字段名为 camelCase（tsMs/latencyMs），仅以 pydantic
别名声明的形式定义一次；T23 编码器以 ``by_alias=True`` 序列化，因此其他地方
不会出现手写的 camelCase 字符串（CR-REG-30）。事件负载绝不包含
堆栈跟踪或凭据（5.3.6，CR-REG-52）。
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.retrieval.citation import Citation

SseEventType = Literal["trace", "token", "citations", "done", "error"]

EVENT_TRACE = "trace"
EVENT_TOKEN = "token"
EVENT_CITATIONS = "citations"
EVENT_DONE = "done"
EVENT_ERROR = "error"


class TraceEventData(BaseModel):
    model_config = ConfigDict(frozen=True, populate_by_name=True)

    node: str = Field(min_length=1)
    status: str = Field(min_length=1)
    detail: str = ""
    ts_ms: float = Field(ge=0, alias="tsMs")


class TokenEventData(BaseModel):
    model_config = ConfigDict(frozen=True)

    text: str


class CitationsEventData(BaseModel):
    model_config = ConfigDict(frozen=True)

    items: tuple[Citation, ...] = ()


class DoneEventData(BaseModel):
    model_config = ConfigDict(frozen=True, populate_by_name=True)

    latency_ms: float = Field(ge=0, alias="latencyMs")


class ErrorEventData(BaseModel):
    model_config = ConfigDict(frozen=True)

    code: str = Field(min_length=1)
    message: str = Field(min_length=1)