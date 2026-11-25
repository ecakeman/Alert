"""ProcessIngestion use case (T20, CR-ING-03~05).

Flow per job: runtime re-validation of the callback URL (T19 rules reused) ->
PROCESSING mark + callback -> load/chunk/embed/write in one transaction ->
READY (chunkCount) or FAILED (errorMsg <= 500, no credentials/full paths) ->
terminal callback. The job table is the reconciliation authority: callback
failures are logged and retried in-implementation, never fatal to the flow
(CR-ING-05).
"""

from __future__ import annotations

import logging

from alert_agent.application.ingestion.ports import (
    CallbackSender,
    Chunker,
    ContentLoader,
    IngestionWriter,
    IngestJobStore,
)
from alert_agent.application.retrieval.ports import Embedder
from alert_agent.domain.ingestion.job import IngestJob
from alert_agent.domain.ingestion.path_rules import PathRuleError, validate_callback_url

LOGGER = logging.getLogger(__name__)


class ProcessIngestion:
    """One persisted job's full execution."""

    def __init__(
        self,
        *,
        store: IngestJobStore,
        loader: ContentLoader,
        chunker: Chunker,
        embedder: Embedder,
        writer: IngestionWriter,
        callbacks: CallbackSender,
        host_allowlist: tuple[str, ...],
    ) -> None:
        self._store = store
        self._loader = loader
        self._chunker = chunker
        self._embedder = embedder
        self._writer = writer
        self._callbacks = callbacks
        self._allowlist = host_allowlist

    async def execute(self, job: IngestJob) -> None:
        doc_id = job.doc_id
        try:
            # Runtime re-validation of the callback URL (T19 rules reused).
            validate_callback_url(job.callback_url, self._allowlist)
        except PathRuleError as exc:
            await self._fail(doc_id, f"INVALID_CALLBACK: {exc}")
            return
        await self._store.mark(doc_id, "PROCESSING")
        await self._callbacks.send(
            job.callback_url, status="PROCESSING", chunk_count=None, error_msg=""
        )
        try:
            content = await self._loader.load(job.file_path)
            chunks = self._chunker.chunk(content)
            embeddings = await self._embed_children(chunks)
            count = await self._writer.write(
                doc_id=doc_id, title=job.file_path, chunks=chunks, embeddings=embeddings
            )
        except Exception as exc:  # noqa: BLE001 -- terminal state must be recorded
            await self._fail(doc_id, f"{type(exc).__name__}: {exc}")
            return
        await self._store.mark(doc_id, "READY", chunk_count=count)
        await self._callbacks.send(
            job.callback_url, status="READY", chunk_count=count, error_msg=""
        )
        LOGGER.info("ingest job %s ready with %d chunks", doc_id, count)

    async def _embed_children(self, chunks) -> list[list[float] | None]:
        children = [chunk for chunk in chunks if not chunk.is_parent]
        vectors = (
            await self._embedder.embed([child.content for child in children])
            if children
            else []
        )
        by_content_id = {id(child): vector for child, vector in zip(children, vectors)}
        # Parents carry no embedding (excluded from the vector leg, CR-REG-24).
        return [
            by_content_id.get(id(chunk)) if not chunk.is_parent else None
            for chunk in chunks
        ]

    async def _fail(self, doc_id: str, message: str) -> None:
        await self._store.mark(doc_id, "FAILED", error_msg=message[:500])
        LOGGER.error("ingest job %s failed: %s", doc_id, message[:200])