"""评分决策模型（design 3.2.1，CR-RAG-09）。

每个候选的分数为 0..10 的整数；``threshold`` 默认为 6，且可
通过配置覆盖；``accepted_ids`` 必须覆盖候选集合，
超出范围的 id 由评分节点丢弃（T17）。``degraded`` 标记
降级回退路径，其默认分数保持在阈值或以上，因此降级
评分仍允许给出答案（CR-RAG-11，无语义矛盾）。
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class GradeItem(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int = Field(ge=0)
    score: int = Field(ge=0, le=10)


class GradeDecision(BaseModel):
    model_config = ConfigDict(frozen=True)

    items: tuple[GradeItem, ...] = ()
    threshold: int = Field(default=6, ge=0, le=10)
    accepted_ids: tuple[int, ...] = ()
    degraded: bool = False