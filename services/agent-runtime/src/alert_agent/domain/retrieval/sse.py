"""SSE event protocol, five event classes (design 3.2.3, CR-RAG-17, 7.4.7).

Wire field names are camelCase (tsMs/latencyMs) declared once as pydantic
aliases; the T23 encoder serializes with ``by_alias=True`` so no hand-written
camelCase strings appear elsewhere (CR-REG-30). Event payloads never carry
stack traces or credentials (5.3.6, CR-REG-52).
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