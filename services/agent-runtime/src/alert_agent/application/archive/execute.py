"""ExecuteArchive use case: the TaskExecutor port implementation for ARCHIVE
tasks (T14, design M6, CR-ARCH-01~09).

Flow: started callback -> dual-artifact precondition (CR-ARCH-01: both
artifacts exist and are readable within this generation, CR-ARCH-02) ->
BUILDING upsert (CR-ARCH-06) -> index extraction through the configured
strategy -> READY (or FAILED with a retained index_error) -> result callback.
The fd registration channel to Go does not exist in the current contract
(system.episode_artifact_refs CHECK accepts INVESTIGATION/DIAGNOSIS_REPORT
only), so the fault-document registration is carried by the cognitive
fault_documents row itself; see the T14 disclosure.
"""

from __future__ import annotations

import logging

from alert_agent.application.archive.ports import (
    ArtifactContentReader,
    FaultDocStore,
    IndexExtractor,
)
from alert_agent.application.shared.ports import Clock
from alert_agent.application.task.executor import TaskExecutor
from alert_agent.application.task.ports import (
    ArtifactStore,
    TaskFailureReport,
    TaskGateway,
    TaskSubmission,
)
from alert_agent.domain.archive.index import FaultDocument, content_hash_of, fd_id_of
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError
from alert_agent.domain.shared.storage_key import parse_storage_key

LOGGER = logging.getLogger(__name__)


class ExecuteArchive(TaskExecutor):
    """ARCHIVE task runner over the artifact store, reader and fault-doc store."""

    def __init__(
        self,
        *,
        gateway: TaskGateway,
        artifact_store: ArtifactStore,
        reader: ArtifactContentReader,
        extractor: IndexExtractor,
        fault_doc_store: FaultDocStore,
        clock: Clock,
    ) -> None:
        self._gateway = gateway
        self._store = artifact_store
        self._reader = reader
        self._extractor = extractor
        self._fault_doc_store = fault_doc_store
        self._clock = clock

    async def execute(self, submission: TaskSubmission) -> None:
        identity = submission.identity
        if identity.task_type != "ARCHIVE":
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                f"ExecuteArchive received a {identity.task_type} task",
            )
        await self._gateway.report_started(identity)
        episode_id = identity.episode_id

        investigation_version = await self._store.latest_version(
            episode_id, "INVESTIGATION"
        )
        report_version = await self._store.latest_version(episode_id, "DIAGNOSIS_REPORT")
        if investigation_version == 0 or report_version == 0:
            raise FatalError(
                ErrorCode.ARTIFACT_MISSING,
                "archive requires both INVESTIGATION and DIAGNOSIS_REPORT to exist (CR-ARCH-01)",
            )
        investigation_record = await self._store.find(
            episode_id, "INVESTIGATION", investigation_version
        )
        report_record = await self._store.find(episode_id, "DIAGNOSIS_REPORT", report_version)
        if investigation_record is None or report_record is None:
            raise FatalError(
                ErrorCode.ARTIFACT_MISSING,
                "registered artifact row is missing although a version exists",
            )
        self._require_generation(investigation_record.storage_key, identity.generation)
        self._require_generation(report_record.storage_key, identity.generation)

        investigation_markdown = await self._reader.read(investigation_record.storage_key)
        report_json = await self._reader.read(report_record.storage_key)

        fd_id = fd_id_of(episode_id)
        await self._fault_doc_store.upsert_building(
            fd_id=fd_id,
            episode_id=episode_id,
            service_id=submission.service_id,
            task_id=identity.task_id,
            generation=identity.generation,
        )
        try:
            extraction = await self._extractor.extract(investigation_markdown, report_json)
        except (RetryableError, FatalError) as exc:
            await self._fault_doc_store.mark_failed(
                fd_id, f"{exc.code.value}: {exc.message}"
            )
            await self._gateway.report_failure(
                TaskFailureReport(
                    identity=identity,
                    retryable=isinstance(exc, RetryableError),
                    error_code=exc.code.value,
                    error_message=exc.message,
                )
            )
            raise
        document = FaultDocument(
            fd_id=fd_id,
            episode_id=episode_id,
            service_id=submission.service_id,
            task_id=identity.task_id,
            generation=identity.generation,
            index=extraction.index,
            content_hash=content_hash_of(extraction.index),
        )
        await self._fault_doc_store.mark_ready(fd_id, document)
        LOGGER.info(
            "fault document %s ready for episode %s via %s",
            fd_id,
            episode_id,
            extraction.strategy,
        )
        # CR-ARCH-09: the fault-document registration to Go has no contract
        # channel (system.episode_artifact_refs CHECK constraint); the ARCHIVE
        # completion is carried by report_result. See the T14 disclosure.
        await self._gateway.report_result(identity)

    @staticmethod
    def _require_generation(storage_key: str, generation: int) -> None:
        """CR-ARCH-02: only artifact versions inside this task's generation."""
        ref = parse_storage_key(storage_key)
        if ref.generation != generation:
            raise FatalError(
                ErrorCode.ARTIFACT_MISSING,
                f"latest artifact belongs to generation {ref.generation},"
                f" not this task's generation {generation}",
            )