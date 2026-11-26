"""RAG 协议 DTO：camelCase 线上格式（design 5.5.3，CR-LAY-10）。

DTO 只负责线上契约；camelCase 与 snake_case 之间的转换通过 pydantic 别名
加映射器层精确发生一次。内部模型保持 snake_case。
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field

_TOP_K_MAX = 20  # 7.4.4


class SearchRequestDto(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    query: str = Field(min_length=1)
    doc_ids: list[str] = Field(default_factory=list, alias="docIds")
    top_k: int = Field(default=5, ge=1, le=_TOP_K_MAX, alias="topK")


class SearchHitDto(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    chunk_id: str = Field(min_length=1, alias="chunkId")
    doc_id: str = Field(min_length=1, alias="docId")
    doc_title: str = ""
    section_path: str | None = Field(default=None, alias="sectionPath")
    page: int | None = Field(default=None, alias="page")
    score: float


class SearchResponseDto(BaseModel):
    items: list[SearchHitDto]