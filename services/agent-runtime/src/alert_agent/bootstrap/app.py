"""FastAPI 应用工厂与进程入口（CR-GW-01，D4）。

``create_app`` 将路由注册表装配到一个 FastAPI 应用上；同一进程通过 lifespan
承载 gRPC 服务器。``main`` 在配置错误时快速失败（非零退出码、脱敏报告、
6.9.2.1），随后将 HTTP 角色交给 uvicorn 的公开 ``Server.run``，使信号处理
保持在 uvicorn 原生的 lifespan 机制中（CR-BOOT-07）。
"""

from __future__ import annotations

import sys

import uvicorn
from fastapi import FastAPI

from alert_agent.bootstrap.lifespan import build_lifespan
from alert_agent.bootstrap.roles import build_http_role
from alert_agent.config import ConfigError, Settings, load_settings
from alert_agent.transport.http.router.registry import build_router


def create_app(settings: Settings | None = None) -> FastAPI:
    resolved = settings if settings is not None else load_settings()
    app = FastAPI(title="alert-agent-runtime", lifespan=build_lifespan(resolved))
    app.state.settings = resolved
    app.state.ready_checks = []
    app.include_router(build_router())
    return app


def main() -> int:
    try:
        settings = load_settings()
    except ConfigError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    app = create_app(settings)
    http_role = build_http_role(settings, app)
    config = uvicorn.Config(
        http_role.app,
        host=http_role.host,
        port=http_role.port,
        log_level=settings.shared.log_level.lower(),
    )
    uvicorn.Server(config).run()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())