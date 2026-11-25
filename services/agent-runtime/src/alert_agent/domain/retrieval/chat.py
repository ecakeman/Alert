"""聊天会话模型（design 3.2.1，CR-RAG-20，7.4.8）。

角色仅限 user/assistant；历史最多保留六轮；消息
本身必须非空。校验集中在此处，HTTP DTO 层
直接复用而非重复定义规则。
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