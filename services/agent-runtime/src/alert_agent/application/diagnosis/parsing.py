"""统一的动作解析路径（design 3.1.2，CR-REG-13）。

LLM 客户端已经解开代码围栏与 JSON（decide 返回 dict）；本模块执行剩余
步骤：信封兼容的提取（接受 {"action": {...}} 包装）然后是可辨识联合的
校验。失败时返回可读的错误字符串，使 decide 节点可以在回退之前携带该
错误追问一次（绝不静默，绝不在深层抛出）。
"""

from __future__ import annotations

from typing import Any

from pydantic import TypeAdapter, ValidationError

from alert_agent.domain.diagnosis.actions import Action

_ACTION_ADAPTER = TypeAdapter(Action)


def parse_action(payload: dict[str, Any]) -> tuple[Action | None, str | None]:
    """校验单个决策载荷。

    成功返回 ``(action, None)``，失败返回 ``(None, error)``，
    调用方在路由到 fallback 之前将错误回喂给模型一次。
    """
    inner = payload.get("action")
    if not isinstance(inner, dict):
        inner = payload
    try:
        return _ACTION_ADAPTER.validate_python(inner), None
    except ValidationError as exc:
        detail = "; ".join(
            f"{'.'.join(str(part) for part in error['loc'])}: {error['msg']}"
            for error in exc.errors()[:3]
        )
        return (
            None,
            f"action validation failed ({exc.error_count()} errors): {detail}",
        )