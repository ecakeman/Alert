"""RetrievedChunk：单条检索结果的唯一表示
（design 3.2.1，CR-RAG-02，CR-REG-04）。

内部模型使用 snake_case；HTTP DTO/映射器在
恰好一处（5.5.3，CR-LAY-10）转换为 camelCase。``leg`` 记录产生该 chunk 的检索
路，因此单路失败仍可区分（CR-RAG-06）。
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

ChunkLeg = Literal["vector", "bm25", "rerank", "parent"]


class RetrievedChunk(BaseModel):
    model_config = ConfigDict(frozen=True)

    chunk_id: str = Field(min_length=1)
    doc_id: str = Field(min_length=1)
    parent_id: str | None = None
    content: str = Field(min_length=1)
    section_path: str | None = None
    page: int | None = Field(default=None, ge=1)
    score: float
    leg: ChunkLeg


class RetrievalResult(BaseModel):
    """一次双路检索的结果；``leg_status`` 报告失败的路
    （CR-RAG-06，CR-REG-25），使索引为空与真实无命中保持可区分。"""

    model_config = ConfigDict(frozen=True)

    chunks: tuple[RetrievedChunk, ...] = ()
    leg_status: dict[str, str] = Field(default_factory=dict)