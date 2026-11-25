"""SystemTaskGateway client: the seven callback RPCs (design 4.3, CR-GW-04).

Every callback reads ``TaskAck.accepted`` as a precondition before any
follow-up action (Q5 lesson); ``accepted=false`` and the Go stable rejection
codes (``LEASE_LOST``/``STALE_GENERATION``, carried as the gRPC
FAILED_PRECONDITION details prefix per ``transport/grpc/errcode.go``) surface
as domain ``FatalError`` so upper layers can trigger threshold-based
termination (CR-TASK-06). UNAVAILABLE and DEADLINE_EXCEEDED get bounded
retries with backoff (CR-REG-14); deterministic rejections never retry. The
call timeout derives from the single ``GATEWAY_RPC_TIMEOUT_MS`` setting
(CR-REG-32). The client never swallows exceptions -- one failed renewal
surfaces immediately and the worker (T08) owns failure counting (CR-REG-44).
"""

from __future__ import annotations

import asyncio
import json
import logging
from datetime import UTC, datetime

import grpc.aio
from grpc import StatusCode

from alert_agent.application.task.ports import (
    ArtifactCallback,
    GatewayAck,
    TaskContextView,
    TaskFailureReport,
)
from alert_agent.domain.shared import ErrorCode, FatalError, TaskIdentity
from alert_agent.infrastructure.grpc.gen.alertflow.system.v1 import common_pb2, task_gateway_pb2
from alert_agent.infrastructure.grpc.gen.alertflow.system.v1.task_gateway_pb2_grpc import (
    SystemTaskGatewayServiceStub,
)

LOGGER = logging.getLogger(__name__)

_RETRYABLE_CODES = frozenset({StatusCode.UNAVAILABLE, StatusCode.DEADLINE_EXCEEDED})

_STABLE_REJECTIONS: dict[str, ErrorCode] = {
    ErrorCode.LEASE_LOST.value: ErrorCode.LEASE_LOST,
    ErrorCode.STALE_GENERATION.value: ErrorCode.STALE_GENERATION,
}


def _identity_message(identity: TaskIdentity) -> common_pb2.TaskIdentity:
    return common_pb2.TaskIdentity(
        task_id=identity.task_id,
        episode_id=identity.episode_id,
        task_type=identity.task_type,
        generation=identity.generation,
        lease_token=identity.lease_token,
    )


class SystemTaskGatewayClient:
    """TaskGateway port implementation over SystemTaskGatewayService (design 4.3)."""

    def __init__(
        self,
        channel: grpc.aio.Channel,
        *,
        timeout_seconds: float,
        max_retries: int,
        backoff_seconds: float | None = None,
    ) -> None:
        self._stub = SystemTaskGatewayServiceStub(channel)
        self._timeout = timeout_seconds
        self._max_retries = max_retries
        self._backoff = timeout_seconds / 4.0 if backoff_seconds is None else backoff_seconds

    async def report_started(self, identity: TaskIdentity) -> GatewayAck:
        request = task_gateway_pb2.ReportTaskStartedRequest(identity=_identity_message(identity))
        return await self._ack("ReportTaskStarted", request)

    async def report_heartbeat(self, identity: TaskIdentity) -> GatewayAck:
        request = task_gateway_pb2.ReportTaskHeartbeatRequest(identity=_identity_message(identity))
        return await self._ack("ReportTaskHeartbeat", request)

    async def register_artifact(self, artifact: ArtifactCallback) -> GatewayAck:
        # Go decodes source_refs as a JSON string array (transport/grpc/mapper.go).
        source_refs = json.dumps(list(artifact.source_refs), ensure_ascii=False).encode("utf-8")
        request = task_gateway_pb2.RegisterTaskArtifactRequest(
            identity=_identity_message(artifact.identity),
            artifact_type=artifact.artifact_type,
            artifact_version=artifact.artifact_version,
            content_hash=artifact.content_hash,
            content_type=artifact.content_type,
            storage_key=artifact.storage_key,
            source_refs=source_refs,
        )
        return await self._ack("RegisterTaskArtifact", request)

    async def report_result(self, identity: TaskIdentity) -> GatewayAck:
        request = task_gateway_pb2.ReportTaskResultRequest(identity=_identity_message(identity))
        return await self._ack("ReportTaskResult", request)

    async def report_failure(self, failure: TaskFailureReport) -> GatewayAck:
        request = task_gateway_pb2.ReportTaskFailureRequest(
            identity=_identity_message(failure.identity),
            retryable=failure.retryable,
            error_code=failure.error_code,
            error_message=failure.error_message,
        )
        return await self._ack("ReportTaskFailure", request)

    async def get_task_context(self, identity: TaskIdentity) -> TaskContextView:
        request = task_gateway_pb2.GetTaskContextRequest(identity=_identity_message(identity))
        response = await self._call("GetTaskContext", request)
        # deadline_at is int64 Unix seconds (Go mapper.go uses time.Time.Unix()).
        deadline_at = datetime.fromtimestamp(response.deadline_at, tz=UTC)
        return TaskContextView(
            input_version_from=response.input_version_from,
            input_version_to=response.input_version_to,
            monitor_watermark=response.monitor_watermark,
            app_event_watermark=response.app_event_watermark,
            deadline_at=deadline_at,
            task_type=response.task_type,
        )

    async def abandon_task(self, identity: TaskIdentity) -> GatewayAck:
        request = task_gateway_pb2.AbandonTaskRequest(identity=_identity_message(identity))
        return await self._ack("AbandonTask", request)

    async def _ack(self, rpc_name: str, request) -> GatewayAck:
        ack = await self._call(rpc_name, request)
        if not ack.accepted:
            raise FatalError(ErrorCode.INPUT_INVALID, f"{rpc_name} rejected: TaskAck.accepted=false")
        return GatewayAck(accepted=True)

    async def _call(self, rpc_name: str, request):
        stub_call = getattr(self._stub, rpc_name)
        attempt = 0
        while True:
            try:
                return await stub_call(request, timeout=self._timeout)
            except grpc.aio.AioRpcError as exc:
                code = exc.code()
                if code in _RETRYABLE_CODES and attempt < self._max_retries:
                    attempt += 1
                    LOGGER.warning(
                        "gateway rpc %s failed with %s (retry %d/%d)",
                        rpc_name,
                        code.name,
                        attempt,
                        self._max_retries,
                    )
                    await asyncio.sleep(self._backoff)
                    continue
                raise self._translate(rpc_name, exc) from exc

    def _translate(self, rpc_name: str, exc: grpc.aio.AioRpcError) -> FatalError:
        details = exc.details() or ""
        stable = _STABLE_REJECTIONS.get(details.split(":", 1)[0].strip())
        if exc.code() == StatusCode.FAILED_PRECONDITION and stable is not None:
            return FatalError(stable, details or f"{rpc_name} rejected by gateway")
        return FatalError(
            ErrorCode.INPUT_INVALID,
            f"{rpc_name} rejected: {exc.code().name}: {details}",
        )