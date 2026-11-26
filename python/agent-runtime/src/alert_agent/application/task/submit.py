"""提交用例：单事务的回执接入（design 4.2，5.1.1）。

接入路径持久化回执后立即返回——任务执行在下游完全异步，使 SubmitTask
的接入保持在 P95 <= 100 ms 预算之内。``thread_id`` 来自唯一的共享规则
（CR-REG-43），因此回执锁定与恢复永远不会漂移。
"""

from __future__ import annotations

from alert_agent.application.shared.ports import Clock
from alert_agent.application.task.ports import AcceptOutcome, ReceiptStore, TaskSubmission
from alert_agent.domain.task import thread_id_of


class SubmitTask:
    """幂等地接受任务提交；排队发生在下游。"""

    def __init__(self, store: ReceiptStore, clock: Clock) -> None:
        self._store = store
        self._clock = clock

    async def execute(self, submission: TaskSubmission) -> AcceptOutcome:
        thread_id = thread_id_of(submission.identity.task_type, submission.identity.episode_id)
        return await self._store.accept(
            submission, thread_id=thread_id, accepted_at=self._clock.now()
        )