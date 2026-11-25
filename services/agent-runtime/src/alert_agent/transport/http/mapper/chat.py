"""Chat DTO <-> domain mapping (T23): the single conversion point for the
chat surface (CR-LAY-10)."""

from __future__ import annotations

from alert_agent.domain.retrieval.chat import ChatRequest
from alert_agent.transport.http.dto.chat import ChatRequestDto


def to_domain_chat_request(dto: ChatRequestDto) -> ChatRequest:
    """camelCase wire DTO -> snake_case domain model (one conversion)."""
    return ChatRequest(
        session_id=dto.session_id,
        message=dto.message,
        history=tuple(turn.model_dump() for turn in dto.history),
        doc_ids=tuple(dto.doc_ids),
        top_k=dto.top_k,
    )