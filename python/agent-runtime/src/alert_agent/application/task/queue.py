"""有界任务队列（design 4.2，CR-TASK-08）。

有界的 ``asyncio.Queue`` 绝不丢弃已接受的任务：入队失败时保留持久化回执
作为"等待恢复"的事实（由恢复扫描重新入队），提交路径报告
``accepted=false`` 而不是伪成功（6.1.3.1）。SubmitTask 路径上的队列元素
携带完整的冻结提交，恢复路径上携带回执引用——回执不持久化租约 token 或
冻结边界（migration 0001），因此恢复的任务无法重建可调用的身份；
参见 TaskRunner。
"""

from __future__ import annotations

import asyncio
from typing import Protocol

from pydantic import BaseModel, ConfigDict, model_validator

from alert_agent.application.task.ports import TaskSubmission, UnfinishedReceipt


class QueuedTask(BaseModel):
    """流经有界队列的一个可执行单元。"""

    model_config = ConfigDict(frozen=True)

    submission: TaskSubmission | None = None
    receipt: UnfinishedReceipt | None = None

    @model_validator(mode="after")
    def _require_exactly_one_source(self) -> QueuedTask:
        if (self.submission is None) == (self.receipt is None):
            raise ValueError("exactly one of submission or receipt must be set")
        return self


class TaskQueue(Protocol):
    """供 worker 循环消费的队列端口（design 4.2）。"""

    def try_enqueue(self, task: QueuedTask) -> bool:
        """非阻塞入队；队列已满时返回 False（绝不丢弃）。"""
        ...

    async def dequeue(self) -> QueuedTask:
        """阻塞直到有一个任务可用。"""
        ...


class BoundedTaskQueue:
    """TaskQueue 的固定容量 asyncio.Queue 实现（CR-TASK-08）。"""

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