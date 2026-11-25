"""Citation model (design 3.2.1, CR-RAG-19).

Numbers are consecutive from 1 in answer order; ``text_excerpt`` is capped at
200 characters; unverifiable citations are dropped by the verify node (T17).
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field

_EXCERPT_CHARS = 200


class Citation(BaseModel):
    model_config = ConfigDict(frozen=True)

    no: int = Field(ge=1)
    chunk_id: str = Field(min_length=1)
    doc_id: str = Field(min_length=1)
    doc_title: str = ""
    section_path: str | None = None
    page: int | None = Field(default=None, ge=1)
    text_excerpt: str = Field(min_length=1, max_length=_EXCERPT_CHARS)