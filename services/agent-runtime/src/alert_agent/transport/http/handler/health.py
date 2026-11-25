"""Health and readiness probes (CR-BOOT-08).

``/health`` reflects process liveness only. ``/ready`` executes the
readiness checks injected into ``app.state`` (gRPC server state now; Postgres
and worker checks land with T05/T08). LLM endpoints are never part of the
readiness probes.
"""

from __future__ import annotations

from collections.abc import Awaitable, Callable

from fastapi import Request
from fastapi.responses import JSONResponse

ReadyCheck = Callable[[], Awaitable[None]]


async def health() -> dict[str, str]:
    return {"status": "alive"}


async def ready(request: Request) -> JSONResponse:
    checks: list[ReadyCheck] = getattr(request.app.state, "ready_checks", [])
    failures: list[str] = []
    for check in checks:
        try:
            await check()
        except Exception as exc:  # noqa: BLE001 -- probe boundary: collect failures, never swallow silently
            failures.append(str(exc) or type(exc).__name__)
    if failures:
        return JSONResponse(status_code=503, content={"status": "unready", "failures": failures})
    return JSONResponse(content={"status": "ready"})