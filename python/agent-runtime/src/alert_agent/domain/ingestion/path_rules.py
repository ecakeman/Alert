"""摄入安全规则，以纯函数实现（design 5.3.4/5.3.5，CR-REG-36）。

路径包含性：``resolve()`` 之后候选路径必须仍位于摄入根目录之内——
``..``、绝对路径、反斜杠和符号链接逃逸一律
拒绝（CR-REG-36，规避 RAG A1）。回调 URL：协议仅限
http/https，且主机必须位于显式允许列表中，该列表在
构造上排除了未列入允许列表的私有网络目标（CR-REG-36，
规避 RAG A2/SSRF）。本模块无 IO（CR-LAY-03）。
"""

from __future__ import annotations

from pathlib import Path
from urllib.parse import urlparse


class PathRuleError(ValueError):
    """任何路径/回调规则违规时抛出；传输层将其映射为
    带稳定错误码的标准 4xx 响应（CR-REG-52）。"""


def resolve_within_root(root: Path, relative: str) -> Path:
    """在 ``root`` 下解析 ``relative``，并严格执行包含性约束。"""
    if not relative or relative.strip() == "":
        raise PathRuleError("file path must not be empty")
    if "\\" in relative:
        raise PathRuleError("file path must not contain backslashes")
    if relative.startswith(("/", "~")):
        raise PathRuleError("file path must be relative to the ingest root")
    if any(part == ".." for part in relative.split("/")):
        raise PathRuleError("file path must not contain '..'")
    root_resolved = root.resolve()
    candidate = (root_resolved / relative).resolve()
    if not candidate.is_relative_to(root_resolved):
        # resolve 之后的符号链接逃逸防御（5.3.4）。
        raise PathRuleError("file path escapes the ingest root")
    return candidate


def validate_callback_url(url: str, host_allowlist: tuple[str, ...]) -> str:
    """校验单个回调 URL：仅限 http/https，主机非空，且主机必须在
    允许列表中（这是唯一的放行通道，因此未列入允许列表的私有
    网络目标无法绕过）。"""
    parsed = urlparse(url)
    if parsed.scheme not in ("http", "https"):
        raise PathRuleError("callback URL scheme must be http or https")
    host = (parsed.hostname or "").lower()
    if not host:
        raise PathRuleError("callback URL host is missing")
    allowed = {entry.strip().lower() for entry in host_allowlist if entry.strip()}
    if not allowed:
        raise PathRuleError("callback host allowlist is empty")
    if host not in allowed:
        raise PathRuleError("callback host is not allowlisted")
    return url