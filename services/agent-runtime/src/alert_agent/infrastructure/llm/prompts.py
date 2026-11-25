"""Prompt asset loading and rendering discipline (design 5.2 #6, 5.3.7, CR-REG-16).

Templates load lazily and read-only from an assets directory and are validated
against a strict pydantic model once. Rendering escapes user/retrieval content
into XML boundary tags with per-section character budgets so prompt injection
payloads cannot cross section boundaries; suspected injection features are
detectable via ``injection_risk`` so callers can emit a ``security`` trace.
"""

from __future__ import annotations

import re
from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.shared import ErrorCode, FatalError

_INJECTION_PATTERNS: tuple[str, ...] = (
    "ignore previous",
    "ignore all previous",
    "disregard the above",
    "system prompt",
    "developer mode",
    "jailbreak",
    "忽略以上",
    "忽略上述",
    "无视以上",
    "<|",
)

_TAG_NAME = re.compile(r"^[a-z][a-z0-9_]*$")


def escape_xml(value: str) -> str:
    """Escape the three XML-significant characters (attribute quotes are not
    used inside prompt sections)."""
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def injection_risk(value: str) -> bool:
    """True when the text hits a prompt-injection feature; callers translate
    this into a ``security`` trace entry (5.3.7)."""
    lowered = value.lower()
    return any(pattern in lowered for pattern in _INJECTION_PATTERNS)


def render_section(tag: str, content: str, *, budget_chars: int) -> str:
    """One escaped, boundary-marked prompt section with a hard character
    budget; truncation is visible to the model, never silent."""
    if _TAG_NAME.match(tag) is None:
        raise ValueError(f"invalid prompt section tag: {tag!r}")
    text = escape_xml(content)
    if len(text) > budget_chars:
        text = text[:budget_chars] + "…[truncated]"
    return f"<{tag}>\n{text}\n</{tag}>"


class TemplateAsset(BaseModel):
    """A validated prompt template (CR-REG-16)."""

    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1, pattern=r"^[a-z][a-z0-9_]*$")
    content: str = Field(min_length=1)


class TemplateLoader:
    """Lazy read-only template loader; templates never come from user input
    and are never written back."""

    def __init__(self, directory: Path) -> None:
        self._directory = directory
        self._cache: dict[str, TemplateAsset] = {}

    def load(self, name: str) -> TemplateAsset:
        if name not in self._cache:
            path = self._directory / f"{name}.md"
            try:
                raw = path.read_text(encoding="utf-8")
            except OSError as exc:
                raise FatalError(
                    ErrorCode.INPUT_INVALID, f"prompt template unavailable: {name}"
                ) from exc
            self._cache[name] = TemplateAsset(name=name, content=raw)
        return self._cache[name]