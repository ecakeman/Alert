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