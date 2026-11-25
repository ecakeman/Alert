"""TaskExecutor 端口：M3 -> M4/M6 的契约锚点（design 7.3）。

执行器接收冻结的 ``TaskSubmission``，并通过其持有的 ``TaskGateway`` 端口
驱动进度、产物与结果/失败回调。worker（bootstrap/worker.py）只围绕此调用
编排生命周期：回执 RUNNING、心跳续期、截止期限强制与终态回执写入。
真正的实现是诊断与归档用例（T13/T14）；该端口被有意设计为唯一的耦合点。
"""

from __future__ import annotations

from typing import Protocol

from alert_agent.application.task.ports import TaskSubmission


class TaskExecutor(Protocol):
    async def execute(self, submission: TaskSubmission) -> None:
        """将一个任务运行至其终态；失败时抛出异常。

        失败必须以领域错误（RetryableError/FatalError）的形式浮出，
        并携带稳定的短错误码（CR-TASK-12/13）；由调用方对它们进行分类。
        """
        ...