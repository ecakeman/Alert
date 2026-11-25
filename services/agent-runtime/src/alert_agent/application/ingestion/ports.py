"""Ingestion ports (T19/T20, CR-ING-02~07).

``IngestJobStore`` is the persisted authority: create/get/mark/delete with
doc-id idempotency (CR-ING-03/07). The execution-side ports (``Chunker``/
``ContentLoader``/``IngestionWriter``/``CallbackSender``) keep the use case
free of technology imports (CR-LAY-02); implementations land in
infrastructure (T20).
"""

from __future__ import annotations

from typing import Protocol

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.ingestion.job import IngestJob, IngestStatus


class IngestJobStore(Protocol):
    async def create(self, job: IngestJob) -> bool:
        """Insert QUEUED; False when doc_id already exists (idempotent
        acceptance, CR-ING-03)."""
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
        """Idempotent removal of the job row (CR-ING-07)."""
        ...

    async def list_queued(self, limit: int = 10) -> list[IngestJob]:
        """Oldest QUEUED jobs for the ingestion worker (T20)."""
        ...


class DocumentChunk(BaseModel):
    """One chunk of the ingestion pipeline; parents carry full sections."""

    model_config = ConfigDict(frozen=True)

    parent_id: str | None = None
    is_parent: bool = False
    chunk_index: int = Field(ge=0)
    content: str = Field(min_length=1)
    section_path: str | None = None
    page: int | None = None


class Chunker(Protocol):
    """Deterministic parent/child chunking (no LLM in the chunking path)."""

    def chunk(self, content: str) -> list[DocumentChunk]: ...


class ContentLoader(Protocol):
    """Reads one document body from the ingest root (path rules enforced)."""

    async def load(self, file_path: str) -> str: ...


class IngestionWriter(Protocol):
    """Single-transaction write of the document + parent/child chunks with
    embeddings; pre-cleaned for idempotent re-runs (CR-ING-03/04)."""

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
    """Three-state callback with the internal token (CR-ING-05); bounded
    retries inside, the job table stays the reconciliation authority."""

    async def send(
        self,
        callback_url: str,
        *,
        status: IngestStatus,
        chunk_count: int | None = None,
        error_msg: str = "",
    ) -> bool: ...