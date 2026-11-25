"""TaskExecutor port: the M3 -> M4/M6 contract anchor (design 7.3).

The executor receives the frozen ``TaskSubmission`` and drives progress,
artifacts and result/failure callbacks through the ``TaskGateway`` port it
holds. The worker (bootstrap/worker.py) only orchestrates lifecycle around
this call: receipt RUNNING, heartbeat renewal, deadline enforcement and the
terminal receipt write. Real implementations are the diagnosis and archive
use cases (T13/T14); the port is intentionally the only coupling point.
"""

from __future__ import annotations

from typing import Protocol

from alert_agent.application.task.ports import TaskSubmission


class TaskExecutor(Protocol):
    async def execute(self, submission: TaskSubmission) -> None:
        """Run one task to its terminal state; raise on failure.

        Failures must surface as domain errors (RetryableError/FatalError)
        with stable short codes (CR-TASK-12/13); the caller classifies them.
        """
        ...