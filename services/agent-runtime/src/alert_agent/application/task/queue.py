"""Bounded task queue (design 4.2, CR-TASK-08).

The bounded ``asyncio.Queue`` never drops an accepted task: enqueue failure
keeps the persisted receipt as the "awaiting recovery" fact (the recovery scan
re-enqueues) and the submit path reports ``accepted=false`` instead of a fake
success (6.1.3.1). Queue elements carry the full frozen submission on the
SubmitTask path and the receipt reference on the recovery path -- receipts do
not persist the lease token or the frozen boundary (migration 0001), so a
recovered task cannot rebuild a callable identity; see TaskRunner.
"""

from __future__ import annotations

import asyncio
from typing import Protocol

from pydantic import BaseModel, ConfigDict, model_validator

from alert_agent.application.task.ports import TaskSubmission, UnfinishedReceipt


class QueuedTask(BaseModel):
    """One executable unit flowing through the bounded queue."""

    model_config = ConfigDict(frozen=True)

    submission: TaskSubmission | None = None
    receipt: UnfinishedReceipt | None = None

    @model_validator(mode="after")
    def _require_exactly_one_source(self) -> QueuedTask:
        if (self.submission is None) == (self.receipt is None):
            raise ValueError("exactly one of submission or receipt must be set")
        return self


class TaskQueue(Protocol):
    """Queue port consumed by the worker loops (design 4.2)."""

    def try_enqueue(self, task: QueuedTask) -> bool:
        """Non-blocking enqueue; False when the queue is full (never drops)."""
        ...

    async def dequeue(self) -> QueuedTask:
        """Block until one task is available."""
        ...


class BoundedTaskQueue:
    """Fixed-capacity asyncio.Queue implementation of TaskQueue (CR-TASK-08)."""

    def __init__(self, capacity: int) -> None:
        self._queue: asyncio.Queue[QueuedTask] = asyncio.Queue(maxsize=capacity)

    def try_enqueue(self, task: QueuedTask) -> bool:
        try:
            self._queue.put_nowait(task)
            return True
        except asyncio.QueueFull:
            return False

    async def dequeue(self) -> QueuedTask:
        return await self._queue.get()