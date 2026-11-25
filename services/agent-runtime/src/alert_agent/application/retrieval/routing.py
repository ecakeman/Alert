"""RAG loop routing (T17, design 3.2.2, CR-RAG-10/12/13/14).

Grade participates in routing: accepted ids drive synthesis; empty acceptance
routes by qtype and remaining budget toward sufficiency or the explicit
refusal. Budget exhaustion is the single termination gate (checked through
``RetrievalBudget.exhausted`` with the per-invocation ``started_at`` from the
runnable config). Verify self-counts and the router only reads the counter,
retrying synthesis once (CR-RAG-14).
"""

from __future__ import annotations

from datetime import datetime

from langgraph.config import RunnableConfig

from alert_agent.application.shared.ports import Clock
from alert_agent.domain.retrieval.state import RetrievalState


class RetrievalRouter:
    """Conditional-edge closures; per-task state arrives via the config."""

    def __init__(self, clock: Clock) -> None:
        self._clock = clock

    def _elapsed_seconds(self, config: RunnableConfig) -> float:
        configurable = (config or {}).get("configurable", {})
        started_at = configurable.get("started_at")
        if isinstance(started_at, datetime):
            return max(0.0, (self._clock.now() - started_at).total_seconds())
        return 0.0

    def _exhausted(self, state: RetrievalState, config: RunnableConfig) -> bool:
        return state.budget.exhausted(
            state, elapsed_seconds=self._elapsed_seconds(config)
        )

    def after_router(self, state: RetrievalState, config: RunnableConfig) -> str:
        if state.qtype == "CHITCHAT":
            return "refuse"
        if state.qtype == "COMPLEX":
            return "decompose"
        return "retrieve"

    def after_grade(self, state: RetrievalState, config: RunnableConfig) -> str:
        if self._exhausted(state, config):
            return "refuse"
        decision = state.grade_decision
        if decision is not None and decision.accepted_ids:
            return "synthesize"
        if state.qtype == "SIMPLE":
            # No accepted chunks and a simple question: refuse now (CR-RAG-10).
            return "refuse"
        return "sufficiency"

    def after_sufficiency(self, state: RetrievalState, config: RunnableConfig) -> str:
        if self._exhausted(state, config):
            return "refuse"
        decision = state.sufficiency
        if decision is not None and decision.verdict == "NEED_MORE":
            # The next hop retrieves with decision.next_query (set by the
            # retrieve node through current_query).
            return "retrieve"
        # ENOUGH without accepted chunks: nothing more to try, refuse.
        return "refuse"

    def after_verify(self, state: RetrievalState, config: RunnableConfig) -> str:
        if state.verified == "FAIL" and state.verify_attempts < 1:
            # One retry maximum, self-counted by the verify node (CR-RAG-14).
            return "synthesize"
        return "done"