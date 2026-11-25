"""任务领域：回执状态机枚举与统一的 thread-id 规则（CR-TASK-04，CR-REG-43）。"""

from __future__ import annotations

from enum import StrEnum

from alert_agent.domain.task.thread_id import thread_id_of


class ReceiptStatus(StrEnum):
    """回执生命周期：ACCEPTED -> RUNNING -> COMPLETED / FAILED（CR-TASK-04）。"""

    ACCEPTED = "ACCEPTED"
    RUNNING = "RUNNING"
    COMPLETED = "COMPLETED"
    FAILED = "FAILED"


class CallbackState(StrEnum):
    """回调投递的记账状态；未确认的结果可重放（CR-TASK-05）。"""

    NOT_SENT = "NOT_SENT"
    SENT = "SENT"
    CONFIRMED = "CONFIRMED"


__all__ = ["CallbackState", "ReceiptStatus", "thread_id_of"]