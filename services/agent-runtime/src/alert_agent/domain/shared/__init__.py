"""Shared domain value objects: task identity, frozen boundary, stable error codes."""

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