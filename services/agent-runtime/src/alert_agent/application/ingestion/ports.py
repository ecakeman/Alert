"""摄取端口（T19/T20，CR-ING-02~07）。

``IngestJobStore`` 是持久化的权威来源：以 doc-id 幂等的方式执行
create/get/mark/delete（CR-ING-03/07）。执行侧端口（``Chunker``/
``ContentLoader``/``IngestionWriter``/``CallbackSender``）使用例层
不依赖任何技术导入（CR-LAY-02）；具体实现位于基础设施层（T20）。
"""

from __future__ import annotations

from typing import Protocol

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.ingestion.job import IngestJob, IngestStatus


class IngestJobStore(Protocol):
    async def create(self, job: IngestJob) -> bool:
        """插入 QUEUED 状态的作业；doc_id 已存在时返回 False（幂等接入，
        CR-ING-03）。"""
        ...

    async def get(self, doc_id: str) -> IngestJob | None: ...

    async def mark(
        self,
        doc_id: str,
        status: IngestStatus,
        *,
        chunk_count: int | None = None,
        error_msg: str = "",
    ) -> bool: ...

    async def delete(self, doc_id: str) -> bool:
        """幂等删除作业行（CR-ING-07）。"""
        ...

    async def list_queued(self, limit: int = 10) -> list[IngestJob]:
        """供摄取 worker 使用的最早 QUEUED 作业列表（T20）。"""
        ...


class DocumentChunk(BaseModel):
    """摄取流水线中的一个分块；父分块携带完整章节。"""

    model_config = ConfigDict(frozen=True)

    parent_id: str | None = None
    is_parent: bool = False
    chunk_index: int = Field(ge=0)
    content: str = Field(min_length=1)
    section_path: str | None = None
    page: int | None = None


class Chunker(Protocol):
    """确定性的父/子分块（分块路径中不使用 LLM）。"""

    def chunk(self, content: str) -> list[DocumentChunk]: ...


class ContentLoader(Protocol):
    """从摄取根目录读取单个文档正文（强制执行路径规则）。"""

    async def load(self, file_path: str) -> str: ...


class IngestionWriter(Protocol):
    """将文档 + 父/子分块及向量写入单个事务；预先清理以支持幂等重跑
    （CR-ING-03/04）。"""

    async def write(
        self,
        *,
        doc_id: str,
        title: str,
        chunks: list[DocumentChunk],
        embeddings: list[list[float] | None],
    ) -> int: ...

    async def delete_document(self, doc_id: str) -> bool: ...


class CallbackSender(Protocol):
    """携带内部 token 的三态回调（CR-ING-05）；内部含有限次重试，
    作业表仍是对账的权威来源。"""

    async def send(
        self,
        callback_url: str,
        *,
        status: IngestStatus,
        chunk_count: int | None = None,
        error_msg: str = "",
    ) -> bool: ...