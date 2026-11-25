"""RetrievalBudget with the single termination entry (design 3.2.2,
CR-RAG-12/13).

``exhausted`` is the only termination gate: hop exhaustion, llm-call
exhaustion, wall-clock deadline or the consecutive zero-new-hits ceiling. The
graph recursion cap is derived from ``max_hops`` and the node count -- no
independent magic constant (CR-RAG-13).
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from pydantic import BaseModel, ConfigDict, Field

if TYPE_CHECKING:
    from alert_agent.domain.retrieval.state import RetrievalState


class RetrievalBudget(BaseModel):
    model_config = ConfigDict(frozen=True)

    max_hops: int = Field(default=3, ge=1)
    max_llm_calls: int = Field(default=12, ge=1)
    deadline_seconds: float = Field(default=90.0, gt=0)
    max_zero_new_hits: int = Field(default=1, ge=0)

    def exhausted(self, state: RetrievalState, *, elapsed_seconds: float) -> bool:
        if state.hop >= self.max_hops:
            return True
        if state.llm_calls >= self.max_llm_calls:
            return True
        if elapsed_seconds >= self.deadline_seconds:
            return True
        return state.zero_new_hits >= self.max_zero_new_hits

    def recursion_limit(self, node_count: int) -> int:
        """LangGraph recursion cap derived from the budget (CR-RAG-13)."""
        return self.max_hops * max(node_count, 1) + node_count + 1