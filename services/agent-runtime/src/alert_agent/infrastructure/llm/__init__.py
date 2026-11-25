"""LLM 适配器（T11）：诊断与 RAG 共享的 OpenAI 兼容客户端。"""

from alert_agent.infrastructure.llm.client import OpenAiCompatibleLlmClient
from alert_agent.infrastructure.llm.prompts import (
    TemplateAsset,
    TemplateLoader,
    escape_xml,
    injection_risk,
    render_section,
)

__all__ = [
    "OpenAiCompatibleLlmClient",
    "TemplateAsset",
    "TemplateLoader",
    "escape_xml",
    "injection_risk",
    "render_section",
]