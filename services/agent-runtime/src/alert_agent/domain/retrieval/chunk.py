"""RetrievedChunk: the single representation of one retrieval result
(design 3.2.1, CR-RAG-02, CR-REG-04).

Internal models are snake_case; the HTTP DTO/mapper converts to camelCase at
exactly one place (5.5.3, CR-LAY-10). ``leg`` records which retrieval leg
produced the chunk so single-leg failures stay distinguishable (CR-RAG-06).
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

ChunkLeg = Literal["vector", "bm25", "rerank", "parent"]


class RetrievedChunk(BaseModel):
    model_config = ConfigDict(frozen=True)

    chunk_id: str = Field(min_length=1)
    doc_id: str = Field(min_length=1)
    parent_id: str | None = None
    content: str = Field(min_length=1)
    section_path: str | None = None
    page: int | None = Field(default=None, ge=1)
    score: float
    leg: ChunkLeg


class RetrievalResult(BaseModel):
    """One dual-leg retrieval outcome; ``leg_status`` reports failed legs
    (CR-RAG-06, CR-REG-25) so empty-index and real-no-hit stay distinguishable."""

    model_config = ConfigDict(frozen=True)

    chunks: tuple[RetrievedChunk, ...] = ()
    leg_status: dict[str, str] = Field(default_factory=dict)