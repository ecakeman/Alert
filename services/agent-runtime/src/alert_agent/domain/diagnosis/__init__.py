"""诊断领域模型（design 3.1，CR-DIAG-01~06/11~14）。

langgraph ``StateGraph(DiagnosisState)`` 使用唯一的 pydantic 状态载体；
节点返回增量字典，列表字段通过 ``ReduceAppend``
（operator.add）归并（CR-DIAG-02，CR-REG-02）。本包无 IO
（CR-LAY-03）；唯一的第三方依赖是 pydantic。
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