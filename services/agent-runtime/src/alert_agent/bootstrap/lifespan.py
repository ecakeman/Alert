"""Single-process dual-server lifespan (D4, design 4.1, CR-BOOT-05~07).

Startup order: settings fail-fast (done by the entrypoint) -> worker role ->
grpc.aio server -> ready checks registered. Shutdown runs in reverse within
the configured budget (<= 20 s): /ready gates off first, gRPC stops with
grace, worker loops drain. Signals stay with uvicorn's native lifespan
handling plus the standard library -- no private-API hacks (CR-BOOT-07).
Postgres readiness probing lands with the persistence batch (T05); worker
readiness with T08.
"""

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

from fastapi import FastAPI

from alert_agent.bootstrap.roles import GrpcRole, WorkerRole, build_grpc_role, build_worker_role
from alert_agent.config import Settings

LOGGER = logging.getLogger(__name__)

ReadyCheck = Callable[[], Awaitable[None]]


def build_lifespan(settings: Settings) -> Callable[[FastAPI], AbstractAsyncContextManager[None]]:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        worker_role = build_worker_role(settings)
        grpc_role = build_grpc_role(settings)
        await worker_role.start()
        await grpc_role.start()
        app.state.ready_checks.append(grpc_started_check(grpc_role))
        try:
            yield
        finally:
            await shutdown(app, worker_role, grpc_role, settings)

    return lifespan


def grpc_started_check(role: GrpcRole) -> ReadyCheck:
    async def check() -> None:
        if not role.started:
            raise RuntimeError("grpc server is not started")

    return check


async def shutdown(
    app: FastAPI,
    worker_role: WorkerRole,
    grpc_role: GrpcRole,
    settings: Settings,
) -> None:
    app.state.ready_checks.clear()
    budget = float(settings.shared.shutdown_timeout_seconds)
    try:
        await asyncio.wait_for(grpc_role.stop(max(budget / 2.0, 0.1)), timeout=budget)
    except TimeoutError:
        LOGGER.exception("grpc server stop exceeded the shutdown budget (%ss)", budget)
    try:
        await asyncio.wait_for(worker_role.stop(), timeout=budget)
    except TimeoutError:
        LOGGER.exception("worker shutdown exceeded the shutdown budget (%ss)", budget)