"""SubmitTaskRequest 与 TaskSubmission 映射：SubmitTask 接口唯一的
proto 与领域模型转换点（CR-GW-03，CR-LAY-10）。

``deadline_at`` 是以**秒**为单位的 int64 Unix 时间戳——已对照构建该字段的
Go 调用方验证：``infrastructure/grpcclient/agent_client.go``
（``DeadlineAt: task.DeadlineAt.Unix()``）与 ``transport/grpc/mapper.go``
（``DeadlineAt: in.DeadlineAt.Unix()``）。解析使用
``datetime.fromtimestamp(seconds, tz=UTC)``。

校验与 proto 契约的数据约束保持一致：未知 task_type 和非正数 generation
由冻结的 TaskIdentity 模型拒绝（7.1.3、7.1.4）；lease_token 原样透传
（7.1.5）；service_id 完全透传（CR-GW-03）。
"""

from __future__ import annotations

from datetime import UTC, datetime

from pydantic import ValidationError

from alert_agent.application.task.ports import AcceptOutcome, TaskSubmission
from alert_agent.domain.shared import ErrorCode, FatalError, FrozenBoundary, TaskIdentity
from alert_agent.infrastructure.grpc.gen.alertflow.agent.v1 import agent_task_pb2


def submission_from_proto(request: agent_task_pb2.SubmitTaskRequest) -> TaskSubmission:
    """将 SubmitTaskRequest 映射为应用层的 TaskSubmission。

    缺失身份、非正数 deadline 以及任何未通过冻结模型校验的载荷，均抛出
    FatalError(INPUT_INVALID)。
    """
    if request.identity is None:
        raise FatalError(ErrorCode.INPUT_INVALID, "missing task identity")
    if request.deadline_at <= 0:
        raise FatalError(
            ErrorCode.INPUT_INVALID,
            "deadline_at must be a positive unix timestamp in seconds",
        )
    deadline_at = datetime.fromtimestamp(request.deadline_at, tz=UTC)
    try:
        return TaskSubmission(
            identity=TaskIdentity(
                task_id=request.identity.task_id,
                episode_id=request.identity.episode_id,
                task_type=request.identity.task_type,
                generation=request.identity.generation,
                lease_token=request.identity.lease_token,
            ),
            boundary=FrozenBoundary(
                input_version_from=request.input_version_from,
                input_version_to=request.input_version_to,
                monitor_watermark=request.monitor_watermark,
                app_event_watermark=request.app_event_watermark,
                deadline_at=deadline_at,
            ),
            service_id=request.service_id,
        )
    except ValidationError as exc:
        raise FatalError(
            ErrorCode.INPUT_INVALID, f"submit task payload failed validation: {_first_error(exc)}"
        ) from exc


def response_from_accept(outcome: AcceptOutcome) -> agent_task_pb2.SubmitTaskResponse:
    """映射受理结果；重复提交仍保持 accepted=true（CR-TASK-01）。"""
    return agent_task_pb2.SubmitTaskResponse(accepted=outcome.accepted)


def _first_error(exc: ValidationError) -> str:
    errors = exc.errors()
    return str(errors[0].get("msg")) if errors else "unknown validation failure"