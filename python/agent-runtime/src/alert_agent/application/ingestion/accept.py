"""摄取接入用例（T19，CR-ING-01/03）。

校验顺序：docId/fileName/title 非空 -> 回调 URL 规则 ->
文件路径包含关系 -> 持久化 QUEUED 状态的作业。任何规则违反都会抛出
``PathRuleError``/``ValueError``，由传输层映射为标准 4xx
（CR-REG-52）。重复的 docId 返回既有作业而不创建新作业
（CR-ING-03）；持久化的作业是权威来源（CR-ING-02）。
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
    """POST /ingest 接入处理；此处不读取任何文件内容。"""

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
            return existing  # 幂等接入（CR-ING-03）
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
        """状态查询读取持久化作业作为权威来源（CR-ING-06）。"""
        return await self._store.get(doc_id)