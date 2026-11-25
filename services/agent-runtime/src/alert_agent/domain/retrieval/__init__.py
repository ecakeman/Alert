"""RAG retrieval domain models (design 3.2, batches T15~T18).

Single pydantic state carrier with first-class decision-chain objects; nodes
return incremental dicts and list fields merge via ``ReduceAppend`` reducers
(CR-REG-01/02). No IO in this package (CR-LAY-03); the only third-party
dependency is pydantic.
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