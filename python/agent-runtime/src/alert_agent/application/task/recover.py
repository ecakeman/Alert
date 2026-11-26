"""恢复用例：重新入队未完成的回执（design 4.1/4.2，CR-TASK-10，5.2.2）。

扫描周期（<= 30 s，``GatewaySettings.recovery_scan_interval_seconds``）
读取 ACCEPTED/RUNNING 的回执并重新入队，使每个未完成的任务在一个扫描周期
内达到可执行或可重试的状态（5.2.2）。同 episode 的执行互斥锁保留在 Go 侧
（CR-TASK-11）——回执只携带统一的 thread_id（CR-REG-43）用于辅助去重，
绝不构成第二个所有权模型。

未确认的结果（COMPLETED + NOT_SENT）在每次扫描中被观察：重放
``report_result`` 需要有效的租约 token，而回执并不持久化它
（migration 0001），因此回调保持 NOT_SENT 并在每次扫描中被重新检查，
直到重新派发的 generation 完成；该限制被记录日志，绝不静默吞掉
（CR-REG-47）。
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
            if self._queue.try_enqueue(QueuedTask(receipt=receipt)):
                reenqueued += 1
                # 队列已满时保留"等待恢复"的事实（CR-TASK-08）；
                # 下一次扫描重试同一回执。
        for receipt in await self._store.list_unconfirmed_results():
            LOGGER.warning(
                "result callback not replayable yet: lease token is not persisted"
                " for task_id=%s generation=%d; callback_state stays NOT_SENT",
                receipt.task_id,
                receipt.generation,
            )
        return reenqueued