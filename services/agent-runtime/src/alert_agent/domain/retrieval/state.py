"""RetrievalState: the single langgraph state carrier for the RAG loop
(design 3.2.1, CR-RAG-01/02/03).

All decision-chain objects are first-class fields (rewrite trace, grade
decision, sufficiency) -- never buried in dicts (CR-RAG-01). ``hits``
accumulates deduplicated chunks by chunk_id (dedup happens in nodes, T17);
``zero_new_hits`` tracks consecutive no-new-hit hops for the termination
gate; ``verify_attempts`` is incremented by the verify node itself and only
read by routing (CR-RAG-14).
"""

from __future__ import annotations

from operator import add
from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.retrieval.budget import RetrievalBudget
from alert_agent.domain.retrieval.chat import ChatTurn
from alert_agent.domain.retrieval.chunk import RetrievedChunk
from alert_agent.domain.retrieval.citation import Citation
from alert_agent.domain.retrieval.grade import GradeDecision
from alert_agent.domain.retrieval.trace import RewriteTrace, SufficiencyDecision, TraceNode

ReduceAppend = add

RetrievalPhase = Literal[
    "REWRITE", "ROUTE", "RETRIEVE", "GRADE", "SYNTHESIZE", "VERIFY", "DONE", "REFUSED"
]
QType = Literal["SIMPLE", "COMPLEX", "CHITCHAT"]
VerifyVerdict = Literal["PASS", "FAIL", "SKIPPED"]


class RetrievalState(BaseModel):
    model_config = ConfigDict(validate_assignment=True)

    # session and question
    session_id: str = Field(min_length=1)
    question: str = Field(min_length=1)
    history: tuple[ChatTurn, ...] = ()
    standalone_query: str = ""
    current_query: str = ""
    qtype: QType | None = None
    sub_questions: tuple[str, ...] = ()

    # retrieval progress (single-hop tracking)
    hop: int = Field(default=0, ge=0)
    llm_calls: int = Field(default=0, ge=0)
    zero_new_hits: int = Field(default=0, ge=0)
    candidates: tuple[RetrievedChunk, ...] = ()
    hits: Annotated[list[RetrievedChunk], ReduceAppend] = Field(default_factory=list)
    new_hit_count: int = 0

    # decision chain
    rewrite_trace: RewriteTrace | None = None
    grade_decision: GradeDecision | None = None
    sufficiency: SufficiencyDecision | None = None

    # synthesis and verification
    answer: str = ""
    citations: tuple[Citation, ...] = ()
    verified: VerifyVerdict | None = None
    verify_attempts: int = Field(default=0, ge=0)

    # observation and budget
    trace: Annotated[list[TraceNode], ReduceAppend] = Field(default_factory=list)
    budget: RetrievalBudget