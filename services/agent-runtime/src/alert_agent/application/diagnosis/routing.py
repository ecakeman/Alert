"""Diagnosis loop routing (design 3.1.4, CR-DIAG-05, CR-REG-07).

Routing consumes phase + action jointly: every router first checks budget
exhaustion, then dispatches by ``action.type``; unknown/absent actions always
reach the explicit fallback (never an implicit continue, never a whole-task
failure). The no-progress ceiling routes to the explicit partial outcome.
"""

from __future__ import annotations

from datetime import datetime

from alert_agent.application.shared.ports import Clock
from alert_agent.domain.diagnosis.state import DiagnosisState


def budget_exhausted(
    state: DiagnosisState, *, now: datetime, started_at: datetime
) -> bool:
    """Unified termination entry (CR-REG-07): rounds, calls and the wall-clock
    ratio of the frozen deadline."""
    if state.tool_rounds >= state.budget.max_tool_rounds:
        return True
    if state.llm_calls >= state.budget.max_llm_calls:
        return True
    total = (state.boundary.deadline_at - started_at).total_seconds()
    if total <= 0:
        return True
    elapsed = (now - started_at).total_seconds()
    return elapsed >= total * state.budget.deadline_ratio


class DiagnosisRouter:
    """Conditional-edge closures over one task's clock and start time."""

    def __init__(self, clock: Clock, started_at: datetime) -> None:
        self._clock = clock
        self._started_at = started_at

    def after_decide(self, state: DiagnosisState) -> str:
        if budget_exhausted(state, now=self._clock.now(), started_at=self._started_at):
            return "finalize_partial"
        action = state.action
        if action is None:
            return "fallback"
        if action.type == "finish":
            return "write"
        return "run_tool"

    def after_tool(self, state: DiagnosisState) -> str:
        if budget_exhausted(state, now=self._clock.now(), started_at=self._started_at):
            return "finalize_partial"
        return "decide"

    def after_fallback(self, state: DiagnosisState) -> str:
        if budget_exhausted(state, now=self._clock.now(), started_at=self._started_at):
            return "finalize_partial"
        if state.no_progress >= state.budget.max_no_progress:
            return "finalize_partial"
        return "decide"