"""DiagnosisState: the single langgraph state carrier (design 3.1.1,
CR-DIAG-01/02/03).

Nodes return incremental dicts and list fields merge through ``ReduceAppend``
(operator.add) so checkpointed replays never duplicate entries. ``phase`` is
assigned only at node boundaries (LOAD/DECIDE/TOOL/WRITE/DONE/FAILED); the
"last action" stays implicit in ``executed_actions`` (CR-DIAG-03). The dead
fields of the reference implementation (investigation_revision /
report_revision / round_marker_hash / current_stage) are deliberately absent
(CR-REG-03).
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