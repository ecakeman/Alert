"""Chat DTO 与领域模型映射（T23）：chat 接口的唯一转换点
（CR-LAY-10）。"""

from __future__ import annotations

from alert_agent.domain.retrieval.chat import ChatRequest
from alert_agent.transport.http.dto.chat import ChatRequestDto


def to_domain_chat_request(dto: ChatRequestDto) -> ChatRequest:
    """camelCase 线上 DTO -> snake_case 领域模型（仅此一次转换）。"""
    return ChatRequest(
        session_id=dto.session_id,
        message=dto.message,
        history=tuple(turn.model_dump() for turn in dto.history),
        doc_ids=tuple(dto.doc_ids),
        top_k=dto.top_k,
    )