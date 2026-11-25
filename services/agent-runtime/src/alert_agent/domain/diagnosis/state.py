"""DiagnosisState：唯一的 langgraph 状态载体（design 3.1.1，
CR-DIAG-01/02/03）。

节点返回增量字典，列表字段通过 ``ReduceAppend``
（operator.add）归并，因此检查点重放不会产生重复条目。``phase``
 仅在节点边界处赋值（LOAD/DECIDE/TOOL/WRITE/DONE/FAILED）；
"上一个动作" 隐式保留在 ``executed_actions`` 中（CR-DIAG-03）。参考实现中的
无效字段（investigation_revision /
report_revision / round_marker_hash / current_stage）被刻意移除
（CR-REG-03）。
"""

from __future__ import annotations

from operator import add
from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.diagnosis.actions import Action
from alert_agent.domain.diagnosis.budget import DiagnosisBudget
from alert_agent.domain.diagnosis.records import (
    ExecutedAction,
    FdHit,
    KnowledgeCallRecord,
    McpToolResult,
    TimelineRecord,
)
from alert_agent.domain.diagnosis.report import DiagnosisReport
from alert_agent.domain.shared import FrozenBoundary, TaskIdentity

ReduceAppend = add

DiagnosisPhase = Literal["LOAD", "DECIDE", "TOOL", "WRITE", "DONE", "FAILED"]


class DiagnosisState(BaseModel):
    model_config = ConfigDict(validate_assignment=True)

    identity: TaskIdentity
    service_id: str = Field(min_length=1)
    service_name: str = Field(min_length=1)
    boundary: FrozenBoundary

    phase: DiagnosisPhase

    budget: DiagnosisBudget
    tool_rounds: int = Field(default=0, ge=0)
    llm_calls: int = Field(default=0, ge=0)
    no_progress: int = Field(default=0, ge=0)

    timeline_records: Annotated[list[TimelineRecord], ReduceAppend] = Field(
        default_factory=list
    )
    search_hits: Annotated[list[FdHit], ReduceAppend] = Field(default_factory=list)
    knowledge: KnowledgeCallRecord = Field(default_factory=KnowledgeCallRecord)
    mcp_results: Annotated[list[McpToolResult], ReduceAppend] = Field(
        default_factory=list
    )
    executed_actions: Annotated[list[ExecutedAction], ReduceAppend] = Field(
        default_factory=list
    )

    action: Action | None = None
    last_error: str | None = None

    investigation_version: int | None = Field(default=None, ge=1)
    report_version: int | None = Field(default=None, ge=1)
    report: DiagnosisReport | None = None
    partial: bool = False