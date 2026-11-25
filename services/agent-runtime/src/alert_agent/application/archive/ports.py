"""Archive use-case ports (design M6, CR-ARCH-01~09).

``IndexExtractor`` produces the four-field index with an explicit strategy
marker (CR-ARCH-04: one clearly named strategy, never mixed semantics);
``ArtifactContentReader`` reads registered artifact bodies through their
storage_key; ``FaultDocStore`` owns the BUILDING -> READY/FAILED lifecycle
(CR-ARCH-06). No technology imports here (CR-LAY-02).
"""

from __future__ import annotations

from typing import Protocol

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.archive.index import FaultDocument, FaultSearchIndex


class ArtifactContentReader(Protocol):
    """Reads one artifact body by storage_key (rooted at the storage root)."""

    async def read(self, storage_key: str) -> str: ...


class ArchiveExtraction(BaseModel):
    """Extraction result with its strategy source (CR-ARCH-04)."""

    model_config = ConfigDict(frozen=True)

    strategy: str = Field(min_length=1)
    index: FaultSearchIndex


class IndexExtractor(Protocol):
    async def extract(
        self, investigation_markdown: str, report_json: str
    ) -> ArchiveExtraction: ...


class FaultDocStore(Protocol):
    """fault_documents lifecycle: BUILDING -> READY / FAILED (CR-ARCH-06)."""

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