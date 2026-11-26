"""恢复用例：重新入队未完成的回执（design 4.1/4.2，CR-TASK-10，5.2.2）。

扫描周期（<= 30 s，``GatewaySettings.recovery_scan_interval_seconds``）
读取 ACCEPTED/RUNNING 的回执并重新入队，使每个未完成的任务在一个扫描周期
内达到可执行或可重试的状态（5.2.2）。同 episode 的执行互斥锁保留在 Go 侧
（CR-TASK-11）——回执只携带统一的 thread_id（CR-REG-43）用于辅助去重，
绝不构成第二个所有权模型。

migration 0006 起回执持久化 lease_token 与冻结边界：六列齐备时
``to_submission`` 重建 TaskSubmission，恢复后的执行复用与首次受理相同的
路径（同一 identity → 同一 token → 心跳/产物/结果原样回传）。六列缺失的
老回执仍以 receipt 形式入队，由 worker 显式降级并记录原因码。

未确认的结果（COMPLETED + NOT_SENT）若 token 与边界齐备则入队重放结果
回调；否则记录"需重派"，绝不静默吞掉（CR-REG-47）。
"""

from __future__ import annotations

import logging

from alert_agent.application.task.ports import ReceiptStore
from alert_agent.application.task.queue import QueuedTask, TaskQueue

LOGGER = logging.getLogger(__name__)


class RecoverUnfinished:
    """读取未完成的回执并重新入队（CR-TASK-10）。"""

    def __init__(self, store: ReceiptStore, queue: TaskQueue) -> None:
        self._store = store
        self._queue = queue

    async def execute(self) -> int:
        reenqueued = 0
        for receipt in await self._store.list_unfinished():
            submission = receipt.to_submission()
            unit = (
                QueuedTask(submission=submission)
                if submission is not None
                else QueuedTask(receipt=receipt)
            )
            if self._queue.try_enqueue(unit):
                reenqueued += 1
            # 队列已满时保留"等待恢复"的事实（CR-TASK-08）；
            # 下一次扫描重试同一回执。

        for receipt in await self._store.list_unconfirmed_results():
            submission = receipt.to_submission()
            if submission is not None:
                if self._queue.try_enqueue(QueuedTask(receipt=receipt)):
                    continue
                LOGGER.warning(
                    "queue full; result replay deferred for task_id=%s generation=%d",
                    receipt.task_id,
                    receipt.generation,
                )
                continue
            LOGGER.warning(
                "result callback not replayable: lease credentials absent"
                " for task_id=%s generation=%d; callback_state stays NOT_SENT",
                receipt.task_id,
                receipt.generation,
            )
        return reenqueued