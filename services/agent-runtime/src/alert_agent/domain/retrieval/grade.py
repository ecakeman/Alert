"""Grade decision models (design 3.2.1, CR-RAG-09).

Per-candidate scores are 0..10 integers; ``threshold`` defaults to 6 and is
configuration-overridable; ``accepted_ids`` must cover the candidate set and
out-of-range ids are dropped by the grade node (T17). ``degraded`` marks the
fallback path whose default scores stay at/above the threshold so a degraded
grade still allows answering (CR-RAG-11, no semantic contradiction).
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