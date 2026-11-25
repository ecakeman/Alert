"""Task domain: receipt state-machine enums and the unified thread-id rule (CR-TASK-04, CR-REG-43)."""

from __future__ import annotations

from enum import StrEnum

from alert_agent.domain.task.thread_id import thread_id_of


class ReceiptStatus(StrEnum):
    """Receipt lifecycle: ACCEPTED -> RUNNING -> COMPLETED / FAILED (CR-TASK-04)."""

    ACCEPTED = "ACCEPTED"
    RUNNING = "RUNNING"
    COMPLETED = "COMPLETED"
    FAILED = "FAILED"


class CallbackState(StrEnum):
    """Callback delivery bookkeeping; unconfirmed results are replayable (CR-TASK-05)."""

    NOT_SENT = "NOT_SENT"
    SENT = "SENT"
    CONFIRMED = "CONFIRMED"


__all__ = ["CallbackState", "ReceiptStatus", "thread_id_of"]