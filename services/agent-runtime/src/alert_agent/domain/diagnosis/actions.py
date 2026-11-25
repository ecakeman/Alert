"""Action discriminated union (design 3.1.2, CR-DIAG-04/05/06).

Five action types, ``extra="forbid"`` so malformed model output fails
validation and routes to the explicit fallback node (CR-DIAG-05). The
discriminator is ``type``; ``parse_action`` (application layer, T12) feeds raw
JSON through ``Action.model_validate``/TypeAdapter after unwrapping.
"""

from __future__ import annotations

from typing import Annotated, Any, Literal

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.diagnosis.report import DiagnosisReport


class ActionBase(BaseModel):
    model_config = ConfigDict(extra="forbid")


class QueryTimelineAction(ActionBase):
    """No parameters on purpose: the range always comes from the frozen
    boundary and the Go side rejects out-of-range reads (CR-DIAG-10)."""

    type: Literal["query_timeline"]


class SearchFaultsAction(ActionBase):
    type: Literal["search_faults"]
    query: str = Field(min_length=1)


class InvokeKnowledgeAction(ActionBase):
    type: Literal["invoke_knowledge_agent"]
    query: str = Field(min_length=1)
    top_k: int = Field(default=8, ge=1, le=20)


class CallMcpToolAction(ActionBase):
    """``name`` is checked against the live tools/list directory before
    execution; ``arguments`` must be an explicit JSON object (CR-DIAG-06)."""

    type: Literal["call_mcp_tool"]
    name: str = Field(min_length=1)
    arguments: dict[str, Any]


class FinishAction(ActionBase):
    type: Literal["finish"]
    report: DiagnosisReport


Action = Annotated[
    QueryTimelineAction
    | SearchFaultsAction
    | InvokeKnowledgeAction
    | CallMcpToolAction
    | FinishAction,
    Field(discriminator="type"),
]