"""RetrievalBudget 与唯一的终止入口（design 3.2.2，
CR-RAG-12/13）。

``exhausted`` 是唯一的终止门控：跳数耗尽、LLM 调用
耗尽、墙钟超时或连续零新命中上限。图的递归上限由
``max_hops`` 与节点数量推导得出——不使用独立的魔法常量（CR-RAG-13）。
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
        """由预算推导出的 LangGraph 递归上限（CR-RAG-13）。"""
        return self.max_hops * max(node_count, 1) + node_count + 1