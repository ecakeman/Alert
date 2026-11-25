"""LLM adapter (T11): OpenAI-compatible client shared by diagnosis and RAG."""

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