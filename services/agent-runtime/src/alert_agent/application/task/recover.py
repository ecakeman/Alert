"""Recovery use case: re-enqueue unfinished receipts (design 4.1/4.2, CR-TASK-10, 5.2.2).

The scan period (<= 30 s, ``GatewaySettings.recovery_scan_interval_seconds``)
reads ACCEPTED/RUNNING receipts and re-enqueues them so every unfinished task
reaches an executable or retryable state within one scan period (5.2.2).
Same-episode execution mutex stays with Go (CR-TASK-11) -- receipts only carry
the unified thread_id (CR-REG-43) for auxiliary dedup, never a second
ownership model.

Unconfirmed results (COMPLETED + NOT_SENT) are observed each scan: replaying
``report_result`` requires a valid lease token, which receipts do not persist
(migration 0001), so the callback stays NOT_SENT and is re-checked each scan
until a re-dispatched generation completes; the limitation is logged, never
silently swallowed (CR-REG-47).
"""

from __future__ import annotations

import logging

from alert_agent.application.task.ports import ReceiptStore
from alert_agent.application.task.queue import QueuedTask, TaskQueue

LOGGER = logging.getLogger(__name__)


class RecoverUnfinished:
    """Read unfinished receipts and re-enqueue them (CR-TASK-10)."""

    def __init__(self, store: ReceiptStore, queue: TaskQueue) -> None:
        self._store = store
        self._queue = queue

    async def execute(self) -> int:
        reenqueued = 0
        for receipt in await self._store.list_unfinished():
            if self._queue.try_enqueue(QueuedTask(receipt=receipt)):
                reenqueued += 1
                # Queue full keeps the "awaiting recovery" fact (CR-TASK-08);
                # the next scan retries the same receipt.
        for receipt in await self._store.list_unconfirmed_results():
            LOGGER.warning(
                "result callback not replayable yet: lease token is not persisted"
                " for task_id=%s generation=%d; callback_state stays NOT_SENT",
                receipt.task_id,
                receipt.generation,
            )
        return reenqueued