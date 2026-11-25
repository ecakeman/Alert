"""Chat 接口 DTO（7.4.8，CR-RAG-20）：校验规则位于 DTO 层；chat 处理器
本身与 T23（SSE）对接。"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

_TOP_K_MAX = 20  # 7.4.4
_HISTORY_MAX = 6  # 7.4.8


class ChatTurnDto(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    role: Literal["user", "assistant"]  # CR-RAG-20：仅允许 user/assistant
    content: str = Field(min_length=1)


class ChatRequestDto(BaseModel):
    """message 非空，history 不超过 6 轮，role 受限（7.4.8）。"""

    model_config = ConfigDict(populate_by_name=True)

    session_id: str = Field(min_length=1, alias="sessionId")
    message: str = Field(min_length=1)
    history: list[ChatTurnDto] = Field(default_factory=list, max_length=_HISTORY_MAX)
    doc_ids: list[str] = Field(default_factory=list, alias="docIds")
    top_k: int = Field(default=5, ge=1, le=_TOP_K_MAX, alias="topK")