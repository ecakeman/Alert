"""Submit use case: single-transaction receipt acceptance (design 4.2, 5.1.1).

The acceptance path persists the receipt and returns immediately -- task
execution is fully asynchronous downstream, keeping SubmitTask acceptance
within the P95 <= 100 ms budget. ``thread_id`` comes from the single shared
rule (CR-REG-43) so receipt locking and recovery never drift.
"""

from __future__ import annotations

from alert_agent.application.shared.ports import Clock
from alert_agent.application.task.ports import AcceptOutcome, ReceiptStore, TaskSubmission
from alert_agent.domain.task import thread_id_of


class SubmitTask:
    """Accept a task submission idempotently; queueing happens downstream."""

    def __init__(self, store: ReceiptStore, clock: Clock) -> None:
        self._store = store
        self._clock = clock

    async def execute(self, submission: TaskSubmission) -> AcceptOutcome:
        thread_id = thread_id_of(submission.identity.task_type, submission.identity.episode_id)
        return await self._store.accept(
            submission, thread_id=thread_id, accepted_at=self._clock.now()
        )