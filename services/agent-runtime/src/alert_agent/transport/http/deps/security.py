"""Internal-token authentication dependency (design 5.3.1, CR-REG-36/51).

Standalone FastAPI dependency: every RAG protocol route must declare its own
dependency on this function -- authentication is never provided sideways by
another route (CR-REG-51, avoiding RAG B1). The comparison is constant time;
a placeholder token is already rejected at startup (T02 fail-fast, R3), so a
missing or empty configured token also fails closed here.
"""

from __future__ import annotations

import hmac

from fastapi import HTTPException, Request

_HEADER = "X-Internal-Token"


async def require_internal_token(request: Request) -> None:
    settings = getattr(request.app.state, "settings", None)
    # internal_token lives in the gateway group (T02 placeholder checks).
    configured = getattr(getattr(settings, "gateway", None), "internal_token", "")
    provided = request.headers.get(_HEADER, "")
    if (
        not configured
        or not provided
        or not hmac.compare_digest(provided.encode("utf-8"), configured.encode("utf-8"))
    ):
        raise HTTPException(
            status_code=401,
            detail={"code": "UNAUTHORIZED", "message": "missing or invalid internal token"},
        )