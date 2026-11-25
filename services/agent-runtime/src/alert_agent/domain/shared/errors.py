"""Stable short error codes and failure classification (design 3.1.6, CR-TASK-12/13).

Codes are short, stable strings (<= 100 chars); exception class names never act
as error codes. Only ``RetryableError`` maps to ``retryable=true`` when
reporting failures (CR-TASK-12); every failure carries a readable message
alongside the code (CR-TASK-13).
"""

from __future__ import annotations

from enum import StrEnum


class ErrorCode(StrEnum):
    STALE_GENERATION = "STALE_GENERATION"
    LEASE_LOST = "LEASE_LOST"
    MODEL_PARSE_ERROR = "MODEL_PARSE_ERROR"
    MODEL_ENDPOINT_UNAVAILABLE = "MODEL_ENDPOINT_UNAVAILABLE"
    KNOWLEDGE_SCHEMA_ERROR = "KNOWLEDGE_SCHEMA_ERROR"
    KNOWLEDGE_INVOCATION_FAILED = "KNOWLEDGE_INVOCATION_FAILED"
    MCP_TOOL_ERROR = "MCP_TOOL_ERROR"
    ARTIFACT_VERSION_CONFLICT = "ARTIFACT_VERSION_CONFLICT"
    ARTIFACT_MISSING = "ARTIFACT_MISSING"
    BOUNDARY_OUT_OF_RANGE = "BOUNDARY_OUT_OF_RANGE"
    INPUT_INVALID = "INPUT_INVALID"


class RetryableError(Exception):
    """Transient failure: network jitter, timeout, temporarily unavailable dependency."""

    def __init__(self, code: ErrorCode, message: str) -> None:
        super().__init__(f"{code.value}: {message}")
        self.code = code
        self.message = message


class FatalError(Exception):
    """Deterministic failure: invalid input, contract mismatch, failed validation."""

    def __init__(self, code: ErrorCode, message: str) -> None:
        super().__init__(f"{code.value}: {message}")
        self.code = code
        self.message = message