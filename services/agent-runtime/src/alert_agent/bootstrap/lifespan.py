"""Single-process dual-server lifespan (D4, design 4.1, CR-BOOT-05~07).

Startup order: settings fail-fast (done by the entrypoint) -> Postgres pool ->
worker role -> grpc.aio server with the SubmitTask servicer (T07) -> ready
checks registered. Shutdown runs in reverse within the configured budget
(<= 20 s): /ready gates off first, gRPC stops with grace, worker loops drain,
then the pool and channel close. Signals stay with uvicorn's native lifespan
handling plus the standard library -- no private-API hacks (CR-BOOT-07).
Worker loop registration lands with the worker batch (T08).
"""

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

from fastapi import FastAPI

from alert_agent.application.task.queue import BoundedTaskQueue
from alert_agent.bootstrap.roles import GrpcRole, WorkerRole, build_grpc_role, build_worker_role
from alert_agent.bootstrap.wiring import (
    build_gateway_channel,
    build_pool,
    build_submit_use_case,
    postgres_ready_check,
)
from alert_agent.bootstrap.worker import EnqueueingSubmit, build_task_worker_runtime
from alert_agent.config import Settings

LOGGER = logging.getLogger(__name__)

ReadyCheck = Callable[[], Awaitable[None]]


def build_lifespan(settings: Settings) -> Callable[[FastAPI], AbstractAsyncContextManager[None]]:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        pool = build_pool(settings)
        await pool.open()
        # One bounded queue shared by the submit path and the consumers
        # (CR-TASK-08); queue-full keeps the receipt as the awaiting-recovery
        # fact and reports accepted=false (6.1.3.1).
        queue = BoundedTaskQueue(settings.gateway.queue_capacity)
        submit = EnqueueingSubmit(build_submit_use_case(pool), queue)
        worker_role = build_worker_role(settings)
        grpc_channel = build_gateway_channel(settings)
        worker_runtime = build_task_worker_runtime(settings, pool, grpc_channel, queue)
        worker_role.tasks.extend(await worker_runtime.start())
        grpc_role = build_grpc_role(settings, submit)
        await grpc_role.start()
        app.state.ready_checks.append(grpc_started_check(grpc_role))
        app.state.ready_checks.append(lambda: postgres_ready_check(pool))
        try:
            yield
        finally:
            await shutdown(app, worker_role, grpc_role, settings)
            await pool.close()
            await grpc_channel.close()

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