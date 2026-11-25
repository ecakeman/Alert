"""SubmitTaskRequest <-> TaskSubmission mapping: the single proto<->domain conversion
point for the SubmitTask surface (CR-GW-03, CR-LAY-10).

``deadline_at`` is an int64 Unix timestamp in **seconds** -- verified against the
Go call sites that build the field: ``infrastructure/grpcclient/agent_client.go``
(``DeadlineAt: task.DeadlineAt.Unix()``) and ``transport/grpc/mapper.go``
(``DeadlineAt: in.DeadlineAt.Unix()``). Parsing uses
``datetime.fromtimestamp(seconds, tz=UTC)``.

Validation mirrors the proto contract data constraints: unknown task_type and
non-positive generation are rejected by the frozen TaskIdentity model (7.1.3,
7.1.4); lease_token is carried through untouched (7.1.5); service_id is passed
through completely (CR-GW-03).
"""

from __future__ import annotations

from datetime import UTC, datetime

from pydantic import ValidationError

from alert_agent.application.task.ports import AcceptOutcome, TaskSubmission
from alert_agent.domain.shared import ErrorCode, FatalError, FrozenBoundary, TaskIdentity
from alert_agent.infrastructure.grpc.gen.alertflow.agent.v1 import agent_task_pb2


def submission_from_proto(request: agent_task_pb2.SubmitTaskRequest) -> TaskSubmission:
    """Map SubmitTaskRequest into the application-level TaskSubmission.

    Raises FatalError(INPUT_INVALID) for missing identity, non-positive
    deadline and any payload failing the frozen-model validation.
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
    """Map the acceptance outcome; duplicate submissions stay accepted=true (CR-TASK-01)."""
    return agent_task_pb2.SubmitTaskResponse(accepted=outcome.accepted)


def _first_error(exc: ValidationError) -> str:
    errors = exc.errors()
    return str(errors[0].get("msg")) if errors else "unknown validation failure"