"""Ingestion acceptance use case (T19, CR-ING-01/03).

Validation order: docId/fileName/title presence -> callback URL rules ->
file-path containment -> persisted QUEUED job. Any rule violation raises
``PathRuleError``/``ValueError`` which the transport maps to a standard 4xx
(CR-REG-52). Duplicate docIds return the existing job without creating a new
one (CR-ING-03); the persisted job is the authority (CR-ING-02).
"""

from __future__ import annotations

from pathlib import Path

from alert_agent.application.ingestion.ports import IngestJobStore
from alert_agent.domain.ingestion.job import IngestJob
from alert_agent.domain.ingestion.path_rules import (
    PathRuleError,
    resolve_within_root,
    validate_callback_url,
)


class AcceptIngestion:
    """POST /ingest acceptance; no file content is read here."""

    def __init__(
        self,
        *,
        store: IngestJobStore,
        ingest_root: Path,
        host_allowlist: tuple[str, ...],
    ) -> None:
        self._store = store
        self._root = ingest_root
        self._allowlist = host_allowlist

    async def execute(
        self,
        *,
        doc_id: str,
        file_name: str,
        title: str,
        callback_url: str,
        file_path: str,
    ) -> IngestJob:
        if not doc_id.strip() or not file_name.strip() or not title.strip():
            raise PathRuleError("docId, fileName and title must be non-empty")
        validate_callback_url(callback_url, self._allowlist)
        resolve_within_root(self._root, file_path)

        existing = await self._store.get(doc_id)
        if existing is not None:
            return existing  # idempotent acceptance (CR-ING-03)
        job = IngestJob(
            doc_id=doc_id,
            status="QUEUED",
            callback_url=callback_url,
            file_path=file_path,
        )
        created = await self._store.create(job)
        if not created:
            existing = await self._store.get(doc_id)
            if existing is None:
                raise RuntimeError("ingest job creation race: doc vanished")
            return existing
        return job

    async def status_of(self, doc_id: str) -> IngestJob | None:
        """Status query reads the persisted job as the authority (CR-ING-06)."""
        return await self._store.get(doc_id)