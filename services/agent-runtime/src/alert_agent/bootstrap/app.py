"""FastAPI application factory and process entrypoint (CR-GW-01, D4).

``create_app`` wires the route registry onto one FastAPI application; the
same process hosts the gRPC server through the lifespan. ``main`` fails fast
on configuration errors (non-zero exit, desensitised report, 6.9.2.1) and
then hands the HTTP role to uvicorn's public ``Server.run`` so signals stay
with uvicorn's native lifespan handling (CR-BOOT-07).
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