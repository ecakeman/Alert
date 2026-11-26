"""RAG 检索领域模型（design 3.2，批次 T15~T18）。

唯一的 pydantic 状态载体，决策链对象为一等字段；节点
返回增量字典，列表字段通过 ``ReduceAppend`` 归并
（CR-REG-01/02）。本包无 IO（CR-LAY-03）；唯一的第三方
依赖是 pydantic。
"""

from alert_agent.domain.retrieval.budget import RetrievalBudget
from alert_agent.domain.retrieval.chat import ChatRequest, ChatTurn
from alert_agent.domain.retrieval.chunk import ChunkLeg, RetrievalResult, RetrievedChunk
from alert_agent.domain.retrieval.citation import Citation
from alert_agent.domain.retrieval.grade import GradeDecision, GradeItem
from alert_agent.domain.retrieval.sse import (
    CitationsEventData,
    DoneEventData,
    ErrorEventData,
    TokenEventData,
    TraceEventData,
)
from alert_agent.domain.retrieval.state import (
    QType,
    RetrievalPhase,
    RetrievalState,
    VerifyVerdict,
)
from alert_agent.domain.retrieval.trace import (
    RewriteTrace,
    SufficiencyDecision,
    TraceNode,
)

__all__ = [
    "ChatRequest",
    "ChatTurn",
    "ChunkLeg",
    "Citation",
    "CitationsEventData",
    "DoneEventData",
    "ErrorEventData",
    "GradeDecision",
    "GradeItem",
    "QType",
    "RetrievalBudget",
    "RetrievalPhase",
    "RetrievalResult",
    "RetrievalState",
    "RetrievedChunk",
    "RewriteTrace",
    "SufficiencyDecision",
    "TokenEventData",
    "TraceEventData",
    "TraceNode",
    "VerifyVerdict",
]