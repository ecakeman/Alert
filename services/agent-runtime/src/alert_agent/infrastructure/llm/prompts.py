"""提示词资产加载与渲染纪律（design 5.2 #6，5.3.7，CR-REG-16）。

模板从资产目录惰性、只读加载，并经严格的 pydantic 模型一次性校验。
渲染时将用户/检索内容转义进 XML 边界标签，并为每个小节设置字符预算，
使提示词注入载荷无法跨越小节边界；可疑注入特征可通过 ``injection_risk``
检测，供调用方发出 ``security`` 追踪。
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
    """转义三个 XML 特殊字符（提示词小节内不使用属性引号）。"""
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def injection_risk(value: str) -> bool:
    """当文本命中提示词注入特征时返回 True；调用方据此产出
    ``security`` 追踪条目（5.3.7）。"""
    lowered = value.lower()
    return any(pattern in lowered for pattern in _INJECTION_PATTERNS)


def render_section(tag: str, content: str, *, budget_chars: int) -> str:
    """生成一个转义、带边界标记且施加硬性字符预算的提示词小节；
    截断对模型可见，绝不静默。"""
    if _TAG_NAME.match(tag) is None:
        raise ValueError(f"invalid prompt section tag: {tag!r}")
    text = escape_xml(content)
    if len(text) > budget_chars:
        text = text[:budget_chars] + "…[truncated]"
    return f"<{tag}>\n{text}\n</{tag}>"


class TemplateAsset(BaseModel):
    """一份通过校验的提示词模板（CR-REG-16）。"""

    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1, pattern=r"^[a-z][a-z0-9_]*$")
    content: str = Field(min_length=1)


class TemplateLoader:
    """惰性只读的模板加载器；模板绝不来自用户输入，也绝不回写。"""

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