"""Chat session models (design 3.2.1, CR-RAG-20, 7.4.8).

Roles are restricted to user/assistant; history is capped at six turns; the
message itself must be non-empty. Validation lives here so the HTTP DTO layer
reuses it instead of duplicating rules.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

Role = Literal["user", "assistant"]

_HISTORY_MAX = 6


class ChatTurn(BaseModel):
    model_config = ConfigDict(frozen=True)

    role: Role
    content: str = Field(min_length=1)


class ChatRequest(BaseModel):
    model_config = ConfigDict(frozen=True, populate_by_name=True)

    session_id: str = Field(min_length=1)
    message: str = Field(min_length=1)
    history: tuple[ChatTurn, ...] = Field(default=(), max_length=_HISTORY_MAX)
    doc_ids: tuple[str, ...] = Field(default=(), alias="docIds")
    top_k: int = Field(default=5, ge=1, le=20, alias="topK")