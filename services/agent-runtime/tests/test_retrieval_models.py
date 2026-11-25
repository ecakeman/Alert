"""RAG 领域模型测试（CR-RAG-09/12/19，7.4.7）：决策链校验、预算门与 camelCase 别名。"""

import pytest
from pydantic import ValidationError

from alert_agent.domain.retrieval.budget import RetrievalBudget
from alert_agent.domain.retrieval.citation import Citation
from alert_agent.domain.retrieval.grade import GradeItem
from alert_agent.domain.retrieval.sse import DoneEventData, TraceEventData
from alert_agent.domain.retrieval.trace import SufficiencyDecision


def test_grade_item_score_bounds() -> None:
    with pytest.raises(ValidationError):
        GradeItem(id=0, score=11)
    with pytest.raises(ValidationError):
        GradeItem(id=-1, score=5)
    assert GradeItem(id=3, score=10).score == 10


def test_sufficiency_requires_next_query_on_need_more() -> None:
    with pytest.raises(ValidationError):
        SufficiencyDecision(verdict="NEED_MORE", next_query="")
    ok = SufficiencyDecision(verdict="NEED_MORE", next_query="连接池指标")
    assert ok.verdict == "NEED_MORE"


def test_budget_exhausted_single_gate() -> None:
    budget = RetrievalBudget(max_hops=3, max_zero_new_hits=1)
    state = _state()
    assert not budget.exhausted(state, elapsed_seconds=0.0)
    assert budget.exhausted(state.model_copy(update={"hop": 3}), elapsed_seconds=0.0)
    assert budget.exhausted(
        state.model_copy(update={"zero_new_hits": 1}), elapsed_seconds=0.0
    )
    assert budget.exhausted(state, elapsed_seconds=90.0)


def test_citation_wire_aliases_are_camel_case() -> None:
    citation = Citation.model_validate(
        {"no": 1, "chunkId": "c1", "docId": "d1", "text_excerpt": "x" * 200}
    )
    dumped = citation.model_dump(by_alias=True)
    assert dumped["chunkId"] == "c1" and dumped["docId"] == "d1"


def test_sse_event_data_camel_case() -> None:
    assert TraceEventData(node="n", status="ok", ts_ms=12.5).model_dump(by_alias=True)["tsMs"] == 12.5
    assert DoneEventData(latency_ms=88.0).model_dump(by_alias=True)["latencyMs"] == 88.0


def _state():
    from alert_agent.domain.retrieval.chat import ChatTurn
    from alert_agent.domain.retrieval.state import RetrievalState

    return RetrievalState(
        session_id="s1",
        question="网关为什么 5xx",
        history=(ChatTurn(role="user", content="之前问过超时"),),
        budget=RetrievalBudget(),
    )