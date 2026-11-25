"""RAG 循环路由（T17，design 3.2.2，CR-RAG-10/12/13/14）。

评分参与路由：被接受的 id 驱动综合生成；空接受时依据 qtype 与剩余预算
路由到充分性判断或显式拒答。预算耗尽是唯一的终止闸门（通过
``RetrievalBudget.exhausted`` 并结合 runnable config 中每次调用的
``started_at`` 检查）。Verify 自行计数，路由器只读取该计数器，
综合生成最多重试一次（CR-RAG-14）。
"""

from __future__ import annotations

from datetime import datetime

from langgraph.config import RunnableConfig

from alert_agent.application.shared.ports import Clock
from alert_agent.domain.retrieval.state import RetrievalState


class RetrievalRouter:
    """条件边闭包；每个任务的状态通过 config 传入。"""

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
            # 没有被接受的分块且是简单问题：立即拒答（CR-RAG-10）。
            return "refuse"
        return "sufficiency"

    def after_sufficiency(self, state: RetrievalState, config: RunnableConfig) -> str:
        if self._exhausted(state, config):
            return "refuse"
        decision = state.sufficiency
        if decision is not None and decision.verdict == "NEED_MORE":
            # 下一跳将使用 decision.next_query 检索（由 retrieve 节点
            # 通过 current_query 设置）。
            return "retrieve"
        # ENOUGH 但没有可接受的分块：没有更多可尝试的，拒答。
        return "refuse"

    def after_verify(self, state: RetrievalState, config: RunnableConfig) -> str:
        if state.verified == "FAIL" and state.verify_attempts < 1:
            # 最多重试一次，由 verify 节点自行计数（CR-RAG-14）。
            return "synthesize"
        return "done"