"""工具结果记录，在进入 DiagnosisState 前完成校验（design 3.1.1）。

每条记录都是严格的 pydantic 模型：timeline 条目必须携带
带时区的 datetime（CR-DIAG-11），故障文档命中保留可追溯的 ``fd_id``
（CR-DIAG-12），知识调用聚合 results/error/called，使失败在下一次
决策提示与报告 unknowns 中保持可见（CR-DIAG-16）。
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
    """单条故障文档命中；``fd_id`` 保持命中可追溯（CR-DIAG-12）。"""

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
    """一轮知识代理调用的聚合记录（CR-DIAG-16；失败即状态）。"""

    model_config = ConfigDict(frozen=True)

    results: tuple[KnowledgeHit, ...] = ()
    error: str | None = None
    called: bool = False


class ExecutedAction(BaseModel):
    """渲染进决策提示的动作表条目（design 3.1.5）。"""

    model_config = ConfigDict(frozen=True)

    round: int = Field(ge=0)
    type: str = Field(min_length=1)
    brief: str = Field(min_length=1)
    ok: bool


class McpToolResult(BaseModel):
    """一次 MCP tools/call 返回的完整 content-block 列表（CR-GW-07）。"""

    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1)
    arguments: dict[str, Any]
    content: tuple[dict[str, Any], ...] = ()
    is_error: bool = False