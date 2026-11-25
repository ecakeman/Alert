"""归档用例端口（design M6，CR-ARCH-01~09）。

``IndexExtractor`` 通过显式的策略标记生成四字段索引（CR-ARCH-04：一个明确
命名的策略，绝不混合语义）；``ArtifactContentReader`` 通过 storage_key
读取已注册的产物正文；``FaultDocStore`` 负责 BUILDING -> READY/FAILED
生命周期（CR-ARCH-06）。此处不包含任何技术导入（CR-LAY-02）。
"""

from __future__ import annotations

from typing import Protocol

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.archive.index import FaultDocument, FaultSearchIndex


class ArtifactContentReader(Protocol):
    """按 storage_key 读取单个产物正文（以存储根目录为基准）。"""

    async def read(self, storage_key: str) -> str: ...


class ArchiveExtraction(BaseModel):
    """携带策略来源的提取结果（CR-ARCH-04）。"""

    model_config = ConfigDict(frozen=True)

    strategy: str = Field(min_length=1)
    index: FaultSearchIndex


class IndexExtractor(Protocol):
    async def extract(
        self, investigation_markdown: str, report_json: str
    ) -> ArchiveExtraction: ...


class FaultDocStore(Protocol):
    """fault_documents 生命周期：BUILDING -> READY / FAILED（CR-ARCH-06）。"""

    async def upsert_building(
        self,
        *,
        fd_id: str,
        episode_id: str,
        service_id: str,
        task_id: str,
        generation: int,
    ) -> str: ...

    async def mark_ready(self, fd_id: str, document: FaultDocument) -> bool: ...

    async def mark_failed(self, fd_id: str, error: str) -> bool: ...


__all__ = [
    "ArchiveExtraction",
    "ArtifactContentReader",
    "FaultDocStore",
    "IndexExtractor",
]