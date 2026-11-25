"""OpenAI 兼容客户端的响应解析（design 5.2 #2，CR-REG-13）。

流水线：剥离代码围栏 -> 空选项守卫 -> JSON 对象提取；schema/pydantic
校验经由 ``validator`` 回调完成，因此失败消息可以回传给模型追问一次
（追问不占用 API 重试预算，规避 RAG L4）。
"""

from __future__ import annotations

import json
from typing import Any


class ModelParseFailure(Exception):
    """内部信号：回复无法解析/校验；在任何致命结局之前触发客户端的
    单次追问路径。"""


def strip_fences(raw: str) -> str:
    """移除包裹的 Markdown 代码围栏（任意语言标签）。"""
    text = raw.strip()
    if text.startswith("```"):
        first_newline = text.find("\n")
        text = text[first_newline + 1 :] if first_newline != -1 else text[3:]
    text = text.removesuffix("```")
    return text.strip()


def parse_json_object(raw: str) -> dict[str, Any]:
    """去围栏 + ``json.loads`` + 对象形状守卫。"""
    text = strip_fences(raw)
    try:
        value = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ModelParseFailure(f"reply is not valid JSON: {exc.msg} at line {exc.lineno}") from exc
    if not isinstance(value, dict):
        raise ModelParseFailure("reply is not a JSON object")
    return value


def extract_choice_content(payload: dict[str, Any]) -> str:
    """chat-completions 响应体 -> 首个 choice 的 content，容忍空的
    choices/content（绝不抛出深层 KeyError，CR-REG-13）。"""
    choices = payload.get("choices")
    if not isinstance(choices, list) or not choices:
        raise ModelParseFailure("chat response carries no choices")
    message = choices[0].get("message")
    content = message.get("content") if isinstance(message, dict) else None
    if not isinstance(content, str) or not content.strip():
        raise ModelParseFailure("chat response message content is empty")
    return content


def extract_stream_delta(chunk: dict[str, Any]) -> str:
    """一个 SSE 流块 -> 增量 token 文本（缺失时返回 ''）。"""
    choices = chunk.get("choices")
    if not isinstance(choices, list) or not choices:
        return ""
    delta = choices[0].get("delta")
    content = delta.get("content") if isinstance(delta, dict) else None
    return content if isinstance(content, str) else ""