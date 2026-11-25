"""ReceiptStore port: idempotent task acceptance (design 4.2/6.2).

Port types speak business fields only (CR-LAY-07); row models stay in
infrastructure. ``accept`` must be atomic via INSERT ... ON CONFLICT
(task_id, generation) DO NOTHING -- never select-then-insert (CR-REG-40, Q1);
FAILED receipts of the same generation are re-accepted with the original
failure record kept (CR-TASK-03); a submission older than the recorded
generation is rejected with ``STALE_GENERATION`` and zero writes (6.1.3.2).
All status transitions carry the generation condition (CR-TASK-04, CR-REG-41).
``mark_finished`` only accepts COMPLETED or FAILED (state machine CR-TASK-04).
"""

from __future__ import annotations

from datetime import datetime
from typing import Protocol

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.shared import FrozenBoundary, TaskIdentity
from alert_agent.domain.task import CallbackState, ReceiptStatus


class TaskSubmission(BaseModel):
    """Application-level task input; mapped 1:1 from SubmitTaskRequest (CR-GW-03)."""

    model_config = ConfigDict(frozen=True)

    identity: TaskIdentity
    boundary: FrozenBoundary
    service_id: str = Field(min_length=1)


class AcceptOutcome(BaseModel):
    """Result of idempotent acceptance (CR-TASK-01)."""

    model_config = ConfigDict(frozen=True)

    accepted: bool
    duplicate: bool


class UnfinishedReceipt(BaseModel):
    """Receipt projection consumed by the recovery use case (CR-TASK-10)."""

    model_config = ConfigDict(frozen=True)

    task_id: str
    generation: int
    task_type: str
    episode_id: str
    service_id: str
    thread_id: str
    status: ReceiptStatus


class ReceiptStore(Protocol):
    async def accept(
        self, submission: TaskSubmission, *, thread_id: str, accepted_at: datetime
    ) -> AcceptOutcome: ...

    async def mark_running(
        self, task_id: str, generation: int, *, started_at: datetime
    ) -> bool: ...

    async def mark_finished(
        self,
        task_id: str,
        generation: int,
        *,
        status: ReceiptStatus,
        error_code: str,
        error_message: str,
        callback_state: CallbackState,
        finished_at: datetime,
    ) -> bool: ...

    async def mark_callback_state(
        self,
        task_id: str,
        generation: int,
        callback_state: CallbackState,
        *,
        updated_at: datetime,
    ) -> bool: ...

    async def list_unfinished(self) -> list[UnfinishedReceipt]: ...

    async def list_unconfirmed_results(self) -> list[UnfinishedReceipt]: ...

class GatewayAck(BaseModel):
    """Protocol-level acknowledgement of one callback (design 4.3).

    ``accepted`` mirrors ``TaskAck.accepted``; the client implementation reads
    it as a precondition for every follow-up action (CR-TASK-06, Q5 lesson).
    """

    model_config = ConfigDict(frozen=True)

    accepted: bool


class ArtifactCallback(BaseModel):
    """RegisterTaskArtifact payload (design 4.3, CR-GW-09)."""

    model_config = ConfigDict(frozen=True)

    identity: TaskIdentity
    artifact_type: str = Field(min_length=1)
    artifact_version: int = Field(gt=0)
    content_hash: str = Field(min_length=1)
    content_type: str = Field(min_length=1)
    storage_key: str = Field(min_length=1)
    source_refs: tuple[str, ...] = ()


class TaskFailureReport(BaseModel):
    """ReportTaskFailure payload; only RetryableError maps to retryable=true (CR-TASK-12/13)."""

    model_config = ConfigDict(frozen=True)

    identity: TaskIdentity
    retryable: bool
    error_code: str = Field(min_length=1, max_length=100)
    error_message: str = Field(min_length=1)


class TaskContextView(BaseModel):
    """GetTaskContext response as business fields (design 4.3).

    ``deadline_at`` is timezone-aware UTC, parsed from the proto int64 Unix
    seconds; callers compare it with the SubmitTask boundary.
    """

    model_config = ConfigDict(frozen=True)

    input_version_from: int
    input_version_to: int
    monitor_watermark: int
    app_event_watermark: int
    deadline_at: datetime
    task_type: str = Field(min_length=1)


class TaskGateway(Protocol):
    """The seven SystemTaskGatewayService callbacks (design 4.3, CR-GW-04).

    Implementations must read ``TaskAck.accepted`` as a precondition for the
    next action and never swallow exceptions: a single failed renewal surfaces
    immediately so the worker can count failures and terminate at threshold
    (CR-REG-44). Deterministic rejections (LEASE_LOST/STALE_GENERATION stable
    codes) raise FatalError; transient ones surface after bounded retries.
    """

    async def report_started(self, identity: TaskIdentity) -> GatewayAck: ...

    async def report_heartbeat(self, identity: TaskIdentity) -> GatewayAck: ...

    async def register_artifact(self, artifact: ArtifactCallback) -> GatewayAck: ...

    async def report_result(self, identity: TaskIdentity) -> GatewayAck: ...

    async def report_failure(self, failure: TaskFailureReport) -> GatewayAck: ...

    async def get_task_context(self, identity: TaskIdentity) -> TaskContextView: ...

    async def abandon_task(self, identity: TaskIdentity) -> GatewayAck: ...