"""AgentTaskService.SubmitTask servicer：薄协议适配（CR-GW-02，CR-LAY-04）。

这里只承载协议映射和提交用例调用——不含队列、DB、心跳或工作流编排。
gRPC 状态码与 ``system-core/internal/transport/grpc/errcode.go`` 中的 Go
映射保持一致：STALE_GENERATION -> FAILED_PRECONDITION，稳定码作为 details
前缀（6.1.3.2）；载荷校验失败 -> INVALID_ARGUMENT；瞬时失败 -> UNAVAILABLE。
"""

from __future__ import annotations

import logging

import grpc.aio
from grpc import StatusCode

from alert_agent.application.task.submit import SubmitTask
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError
from alert_agent.infrastructure.grpc.gen.alertflow.agent.v1 import (
    agent_task_pb2,
    agent_task_pb2_grpc,
)
from alert_agent.transport.grpc.mapper import response_from_accept, submission_from_proto

LOGGER = logging.getLogger(__name__)


class AgentTaskServicer(agent_task_pb2_grpc.AgentTaskServiceServicer):
    """为 Go 控制面提供 AgentTaskService.SubmitTask 服务（design 4.2）。"""

    def __init__(self, submit: SubmitTask) -> None:
        self._submit = submit

    async def SubmitTask(
        self, request: agent_task_pb2.SubmitTaskRequest, context: grpc.aio.ServicerContext
    ) -> agent_task_pb2.SubmitTaskResponse:
        try:
            submission = submission_from_proto(request)
        except FatalError as exc:
            await self._abort(context, StatusCode.INVALID_ARGUMENT, exc)
            return agent_task_pb2.SubmitTaskResponse()
        try:
            outcome = await self._submit.execute(submission)
        except FatalError as exc:
            code = (
                StatusCode.FAILED_PRECONDITION
                if exc.code is ErrorCode.STALE_GENERATION
                else StatusCode.INVALID_ARGUMENT
            )
            await self._abort(context, code, exc)
            return agent_task_pb2.SubmitTaskResponse()
        except RetryableError as exc:
            await self._abort(context, StatusCode.UNAVAILABLE, exc)
            return agent_task_pb2.SubmitTaskResponse()
        except Exception:
            LOGGER.exception("submit task acceptance failed unexpectedly")
            await context.abort(StatusCode.INTERNAL, "INTERNAL: submit task acceptance failed")
            return agent_task_pb2.SubmitTaskResponse()
        return response_from_accept(outcome)

    @staticmethod
    async def _abort(
        context: grpc.aio.ServicerContext, code: StatusCode, exc: FatalError | RetryableError
    ) -> None:
        """稳定码保留为 details 前缀（遵循 Go errcode.go 约定）。"""
        await context.abort(code, f"{exc.code.value}: {exc.message}")