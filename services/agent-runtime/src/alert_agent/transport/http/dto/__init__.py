"""HTTP DTO package: camelCase wire contracts for the RAG protocol surface."""

from alert_agent.transport.http.dto.chat import ChatRequestDto, ChatTurnDto
from alert_agent.transport.http.dto.search import (
    SearchHitDto,
    SearchRequestDto,
    SearchResponseDto,
)

__all__ = [
    "ChatRequestDto",
    "ChatTurnDto",
    "SearchHitDto",
    "SearchRequestDto",
    "SearchResponseDto",
]