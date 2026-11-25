"""DeleteIngestion 用例（T20，CR-ING-07）。

幂等删除：删除不存在的 docId 仍然视为成功。文档（连同其分块，由迁移外键级联删除）
与作业行一并移除；作业行的删除是接入侧的最终事实。
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