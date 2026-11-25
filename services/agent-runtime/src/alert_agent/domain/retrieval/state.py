"""RetrievalState：RAG 循环唯一的 langgraph 状态载体
（design 3.2.1，CR-RAG-01/02/03）。

所有决策链对象均为一等字段（重写轨迹、评分
决策、充分性判断）——绝不埋在字典中（CR-RAG-01）。``hits``
 按 chunk_id 累积去重后的 chunk（去重在节点中完成，T17）；
``zero_new_hits`` 追踪连续无新命中的跳数，供终止
门控使用；``verify_attempts`` 由 verify 节点自身递增，仅由
路由读取（CR-RAG-14）。
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

    # 会话与问题
    session_id: str = Field(min_length=1)
    question: str = Field(min_length=1)
    history: tuple[ChatTurn, ...] = ()
    standalone_query: str = ""
    current_query: str = ""
    qtype: QType | None = None
    sub_questions: tuple[str, ...] = ()

    # 检索进度（单跳跟踪）
    hop: int = Field(default=0, ge=0)
    llm_calls: int = Field(default=0, ge=0)
    zero_new_hits: int = Field(default=0, ge=0)
    candidates: tuple[RetrievedChunk, ...] = ()
    hits: Annotated[list[RetrievedChunk], ReduceAppend] = Field(default_factory=list)
    new_hit_count: int = 0

    # 决策链
    rewrite_trace: RewriteTrace | None = None
    grade_decision: GradeDecision | None = None
    sufficiency: SufficiencyDecision | None = None

    # 合成与核验
    answer: str = ""
    citations: tuple[Citation, ...] = ()
    verified: VerifyVerdict | None = None
    verify_attempts: int = Field(default=0, ge=0)

    # 观测与预算
    trace: Annotated[list[TraceNode], ReduceAppend] = Field(default_factory=list)
    budget: RetrievalBudget