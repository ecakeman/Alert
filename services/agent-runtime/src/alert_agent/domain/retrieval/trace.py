"""Trace and decision-chain models (design 3.2.1, CR-REG-12).

``ts_ms`` fields carry real elapsed milliseconds -- never a constant zero.
``SufficiencyDecision`` requires ``next_query`` exactly when the verdict is
NEED_MORE so the next hop always has a concrete query.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator


class TraceNode(BaseModel):
    model_config = ConfigDict(frozen=True)

    node: str = Field(min_length=1)
    status: str = Field(min_length=1)
    detail: str = ""
    ts_ms: float = Field(ge=0)


class RewriteTrace(BaseModel):
    model_config = ConfigDict(frozen=True)

    original_question: str = Field(min_length=1)
    standalone_query: str = Field(min_length=1)
    sub_questions: tuple[str, ...] = ()
    ts_ms: float = Field(ge=0)


class SufficiencyDecision(BaseModel):
    model_config = ConfigDict(frozen=True)

    verdict: Literal["ENOUGH", "NEED_MORE"]
    missing: str = ""
    next_query: str = ""

    @model_validator(mode="after")
    def _next_query_required_for_need_more(self) -> SufficiencyDecision:
        if self.verdict == "NEED_MORE" and not self.next_query:
            raise ValueError("next_query is required when verdict is NEED_MORE")
        return self