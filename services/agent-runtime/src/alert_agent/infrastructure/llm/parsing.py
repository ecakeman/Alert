"""Response parsing for the OpenAI-compatible client (design 5.2 #2, CR-REG-13).

Pipeline: strip code fences -> empty-choices guard -> JSON object extraction;
schema/pydantic validation happens through ``validator`` callbacks so a
failure message can be fed back to the model for one re-ask (the re-ask never
consumes the API retry budget, avoiding RAG L4).
"""

from __future__ import annotations

import json
from typing import Any


class ModelParseFailure(Exception):
    """Internal signal: the reply could not be parsed/validated; triggers the
    single re-ask path in the client before any fatal outcome."""


def strip_fences(raw: str) -> str:
    """Remove surrounding Markdown code fences (any language tag)."""
    text = raw.strip()
    if text.startswith("```"):
        first_newline = text.find("\n")
        text = text[first_newline + 1 :] if first_newline != -1 else text[3:]
    text = text.removesuffix("```")
    return text.strip()


def parse_json_object(raw: str) -> dict[str, Any]:
    """Fence stripping + ``json.loads`` + object shape guard."""
    text = strip_fences(raw)
    try:
        value = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ModelParseFailure(f"reply is not valid JSON: {exc.msg} at line {exc.lineno}") from exc
    if not isinstance(value, dict):
        raise ModelParseFailure("reply is not a JSON object")
    return value


def extract_choice_content(payload: dict[str, Any]) -> str:
    """Chat-completions body -> first choice content, tolerant of empty
    choices/content (never raises deep KeyErrors, CR-REG-13)."""
    choices = payload.get("choices")
    if not isinstance(choices, list) or not choices:
        raise ModelParseFailure("chat response carries no choices")
    message = choices[0].get("message")
    content = message.get("content") if isinstance(message, dict) else None
    if not isinstance(content, str) or not content.strip():
        raise ModelParseFailure("chat response message content is empty")
    return content


def extract_stream_delta(chunk: dict[str, Any]) -> str:
    """One SSE stream chunk -> incremental token text ('' when absent)."""
    choices = chunk.get("choices")
    if not isinstance(choices, list) or not choices:
        return ""
    delta = choices[0].get("delta")
    content = delta.get("content") if isinstance(delta, dict) else None
    return content if isinstance(content, str) else ""