"""Unified action parsing path (design 3.1.2, CR-REG-13).

The LLM client already unwraps fences and JSON (decide returns a dict); this
module performs the remaining steps: envelope-compatible extraction (a
{"action": {...}} wrapper is accepted) then discriminated-union validation.
Failures return a readable error string so the decide node can re-ask once
with the error before falling back (never silent, never raising deep).
"""

from __future__ import annotations

from typing import Any

from pydantic import TypeAdapter, ValidationError

from alert_agent.domain.diagnosis.actions import Action

_ACTION_ADAPTER = TypeAdapter(Action)


def parse_action(payload: dict[str, Any]) -> tuple[Action | None, str | None]:
    """Validate one decision payload.

    Returns ``(action, None)`` on success or ``(None, error)`` so the caller
    feeds the error back to the model once before routing to fallback.
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