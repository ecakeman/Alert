"""Search DTO <-> domain mapping: the single conversion point (CR-LAY-10).

camelCase wire format lives only in DTOs; this mapper is the only place that
converts between the wire and the snake_case internal models (5.5.3).
"""

from __future__ import annotations

from alert_agent.domain.retrieval.chunk import RetrievalResult
from alert_agent.transport.http.dto.search import (
    SearchHitDto,
    SearchRequestDto,
    SearchResponseDto,
)


def to_search_params(dto: SearchRequestDto) -> dict:
    """Wire DTO -> searcher keyword arguments (snake_case)."""
    doc_ids = tuple(dto.doc_ids) if dto.doc_ids else None
    return {
        "query": dto.query,
        "doc_ids": doc_ids,
        "vector_top_k": dto.top_k,
        "fusion_top_k": dto.top_k,
    }


def chunk_to_hit(chunk) -> SearchHitDto:
    """RetrievedChunk -> wire hit DTO (no dict shuffling in between)."""
    return SearchHitDto(
        chunkId=chunk.chunk_id,
        docId=chunk.doc_id,
        doc_title="",
        sectionPath=chunk.section_path,
        page=chunk.page,
        score=chunk.score,
    )


def response_from(result: RetrievalResult) -> SearchResponseDto:
    return SearchResponseDto(
        items=[chunk_to_hit(chunk) for chunk in result.chunks]
    )