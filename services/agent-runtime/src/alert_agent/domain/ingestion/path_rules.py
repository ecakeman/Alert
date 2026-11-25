"""Ingestion safety rules as pure functions (design 5.3.4/5.3.5, CR-REG-36).

Path containment: after ``resolve()`` the candidate must stay inside the
ingest root -- ``..``, absolute paths, backslashes and symlink escapes are
rejected (CR-REG-36, avoiding RAG A1). Callback URLs: scheme limited to
http/https and the host must be on the explicit allowlist, which by
construction excludes non-allowlisted private-network targets (CR-REG-36,
avoiding RAG A2/SSRF). No IO in this module (CR-LAY-03).
"""

from __future__ import annotations

from pathlib import Path
from urllib.parse import urlparse


class PathRuleError(ValueError):
    """Raised for any path/callback rule violation; the transport maps it to
    a standard 4xx with a stable code (CR-REG-52)."""


def resolve_within_root(root: Path, relative: str) -> Path:
    """Resolve ``relative`` under ``root`` with strict containment."""
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
        # Symlink escape defence after resolve (5.3.4).
        raise PathRuleError("file path escapes the ingest root")
    return candidate


def validate_callback_url(url: str, host_allowlist: tuple[str, ...]) -> str:
    """Validate one callback URL: http/https only, non-empty host, host on the
    allowlist (which is the single pass channel, so non-allowlisted private
    network targets cannot slip through)."""
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