"""Tool-result records validated before entering DiagnosisState (design 3.1.1).

Every record is a strict pydantic model: timeline entries must carry
timezone-aware datetimes (CR-DIAG-11), fault-doc hits keep traceable ``fd_id``
(CR-DIAG-12), knowledge calls aggregate results/error/called so failures stay
visible in the next decision prompt and in report unknowns (CR-DIAG-16).
"""

from __future__ import annotations

from datetime import datetime
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator

from alert_agent.domain.knowledge.models import KnowledgeHit


def _require_timezone(value: datetime) -> datetime:
    if value.tzinfo is None or value.tzinfo.utcoffset(value) is None:
        raise ValueError("datetime must carry an explicit timezone")
    return value


class TimelineRecord(BaseModel):
    model_config = ConfigDict(frozen=True)

    record_type: Literal["MONITOR_POINT", "APP_EVENT"]
    record_id: str = Field(min_length=1)
    event_time: datetime
    received_at: datetime
    source: str = Field(min_length=1)
    content: dict[str, Any]

    @field_validator("event_time", "received_at")
    @classmethod
    def _timezone_aware(cls, value: datetime) -> datetime:
        return _require_timezone(value)


class FdHit(BaseModel):
    """One fault-document hit; ``fd_id`` keeps the hit traceable (CR-DIAG-12)."""

    model_config = ConfigDict(frozen=True)

    fd_id: str = Field(min_length=1)
    episode_id: str = Field(min_length=1)
    service_id: str = Field(min_length=1)
    summary: str = Field(min_length=1)
    matched: str = Field(min_length=1)
    score: float
    diagnosis: list[str] = Field(default_factory=list)

    @field_validator("diagnosis", mode="before")
    @classmethod
    def _scalar_to_list(cls, value: Any) -> Any:
        if value is None:
            return []
        if isinstance(value, str):
            return [value]
        return value


class KnowledgeCallRecord(BaseModel):
    """Aggregate of one knowledge-agent round (CR-DIAG-16; failure is state)."""

    model_config = ConfigDict(frozen=True)

    results: tuple[KnowledgeHit, ...] = ()
    error: str | None = None
    called: bool = False


class ExecutedAction(BaseModel):
    """Action-table entry rendered into the decision prompt (design 3.1.5)."""

    model_config = ConfigDict(frozen=True)

    round: int = Field(ge=0)
    type: str = Field(min_length=1)
    brief: str = Field(min_length=1)
    ok: bool


class McpToolResult(BaseModel):
    """Full content-block list returned by one MCP tools/call (CR-GW-07)."""

    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1)
    arguments: dict[str, Any]
    content: tuple[dict[str, Any], ...] = ()
    is_error: bool = False