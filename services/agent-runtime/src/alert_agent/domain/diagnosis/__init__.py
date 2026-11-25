"""Diagnosis domain models (design 3.1, CR-DIAG-01~06/11~14).

Single pydantic state carrier for the langgraph ``StateGraph(DiagnosisState)``;
nodes return incremental dicts and list fields merge via ``ReduceAppend``
(operator.add) reducers (CR-DIAG-02, CR-REG-02). No IO in this package
(CR-LAY-03); the only third-party dependency is pydantic.
"""

from alert_agent.domain.diagnosis.actions import (
    Action,
    CallMcpToolAction,
    FinishAction,
    InvokeKnowledgeAction,
    QueryTimelineAction,
    SearchFaultsAction,
)
from alert_agent.domain.diagnosis.budget import DiagnosisBudget
from alert_agent.domain.diagnosis.records import (
    ExecutedAction,
    FdHit,
    KnowledgeCallRecord,
    McpToolResult,
    TimelineRecord,
)
from alert_agent.domain.diagnosis.report import DiagnosisReport
from alert_agent.domain.diagnosis.state import DiagnosisPhase, DiagnosisState

__all__ = [
    "Action",
    "CallMcpToolAction",
    "DiagnosisBudget",
    "DiagnosisPhase",
    "DiagnosisReport",
    "DiagnosisState",
    "ExecutedAction",
    "FdHit",
    "FinishAction",
    "InvokeKnowledgeAction",
    "KnowledgeCallRecord",
    "McpToolResult",
    "QueryTimelineAction",
    "SearchFaultsAction",
    "TimelineRecord",
]