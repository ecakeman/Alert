"""Ingestion ports (T19, CR-ING-02/03/06).

``IngestJobStore`` is the persisted authority: create/get/mark with doc-id
idempotency (CR-ING-03). No technology imports here (CR-LAY-02); the
Postgres implementation lands in the same batch.
"""

from __future__ import annotations

from typing import Protocol

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

    async def list_queued(self, limit: int = 10) -> list[IngestJob]:
        """Oldest QUEUED jobs for the ingestion worker (T20)."""
        ...