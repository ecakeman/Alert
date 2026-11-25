"""DeleteIngestion use case (T20, CR-ING-07).

Idempotent removal: deleting a non-existent docId is still a success. The
document (with its chunks, cascaded by the migration FK) and the job row are
removed together; the job row deletion is the acceptance-side truth.
"""

from __future__ import annotations

import logging

from alert_agent.application.ingestion.ports import IngestionWriter, IngestJobStore

LOGGER = logging.getLogger(__name__)


class DeleteIngestion:
    def __init__(self, *, store: IngestJobStore, writer: IngestionWriter) -> None:
        self._store = store
        self._writer = writer

    async def execute(self, doc_id: str) -> bool:
        doc_deleted = await self._writer.delete_document(doc_id)
        job_deleted = await self._store.delete(doc_id)
        deleted = doc_deleted or job_deleted
        LOGGER.info("ingest deletion for %s: doc=%s job=%s", doc_id, doc_deleted, job_deleted)
        return deleted