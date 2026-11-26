"""稳定的短错误码与失败分类（design 3.1.6，CR-TASK-12/13）。

错误码是简短、稳定的字符串（<= 100 字符）；异常类名绝不能
充当错误码。上报失败时仅 ``RetryableError`` 映射为
``retryable=true``（CR-TASK-12）；每条失败在错误码之外
都附带可读消息（CR-TASK-13）。
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
    """瞬态失败：网络抖动、超时、依赖暂时不可用。"""

    def __init__(self, code: ErrorCode, message: str) -> None:
        super().__init__(f"{code.value}: {message}")
        self.code = code
        self.message = message


class FatalError(Exception):
    """确定性失败：无效输入、契约不匹配、校验失败。"""

    def __init__(self, code: ErrorCode, message: str) -> None:
        super().__init__(f"{code.value}: {message}")
        self.code = code
        self.message = message