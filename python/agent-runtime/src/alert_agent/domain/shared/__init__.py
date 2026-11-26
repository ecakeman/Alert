"""共享领域值对象：任务身份、冻结边界、稳定错误码。"""

from __future__ import annotations

from alert_agent.domain.shared.errors import ErrorCode, FatalError, RetryableError
from alert_agent.domain.shared.identity import FrozenBoundary, TaskIdentity, TaskType

__all__ = [
    "ErrorCode",
    "FatalError",
    "FrozenBoundary",
    "RetryableError",
    "TaskIdentity",
    "TaskType",
]