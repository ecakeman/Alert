"""健康与就绪探针（CR-BOOT-08）。

``/health`` 仅反映进程存活状态。``/ready`` 执行注入到 ``app.state`` 的
就绪检查（当前为 gRPC 服务器状态；Postgres 与 worker 检查随 T05/T08 落地）。
LLM 端点绝不属于就绪探针。
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
        except Exception as exc:  # noqa: BLE001 -- 探针边界：收集失败项，绝不静默吞掉
            failures.append(str(exc) or type(exc).__name__)
    if failures:
        return JSONResponse(status_code=503, content={"status": "unready", "failures": failures})
    return JSONResponse(content={"status": "ready"})