"""诊断循环路由（design 3.1.4，CR-DIAG-05，CR-REG-07）。

路由联合消费 phase + action：每个路由器先检查预算耗尽，再按
``action.type`` 分发；未知/缺失的动作总是到达显式 fallback
（绝不隐式继续，也绝不导致整个任务失败）。无进展上限路由到显式的
部分结果。
"""

from __future__ import annotations

from datetime import datetime

from alert_agent.application.shared.ports import Clock
from alert_agent.domain.diagnosis.state import DiagnosisState


def budget_exhausted(
    state: DiagnosisState, *, now: datetime, started_at: datetime
) -> bool:
    """统一的终止入口（CR-REG-07）：轮次、调用次数与冻结期限的
    挂钟时间比例。"""
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
    """基于单个任务的时钟与开始时间的条件边闭包。"""

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