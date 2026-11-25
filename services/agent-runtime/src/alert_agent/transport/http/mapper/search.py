"""Search DTO 与领域模型映射：唯一转换点（CR-LAY-10）。

camelCase 线上格式只存在于 DTO 中；本映射器是线上格式与 snake_case 内部
模型之间唯一的转换位置（5.5.3）。
"""

from __future__ import annotations

from alert_agent.domain.retrieval.chunk import RetrievalResult
from alert_agent.transport.http.dto.search import (
    SearchHitDto,
    SearchRequestDto,
    SearchResponseDto,
)


def to_search_params(dto: SearchRequestDto) -> dict:
    """线上 DTO -> 检索器关键字参数（snake_case）。"""
    doc_ids = tuple(dto.doc_ids) if dto.doc_ids else None
    return {
        "query": dto.query,
        "doc_ids": doc_ids,
        "vector_top_k": dto.top_k,
        "fusion_top_k": dto.top_k,
    }


def chunk_to_hit(chunk) -> SearchHitDto:
    """RetrievedChunk -> 线上命中 DTO（中间不做字典搬运）。"""
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