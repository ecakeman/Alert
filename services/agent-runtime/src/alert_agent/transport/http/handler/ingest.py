"""Ingest acceptance and status handlers (T19, CR-LAY-04, CR-REG-52).

Thin handlers only: bind, validate, call the acceptance use case, map DTOs,
write the response. Rule violations map to standard 4xx with the stable code
in the body; unknown docs map to 404 (status query reads the persisted job as
the authority, CR-ING-06).
"""

from __future__ import annotations

from fastapi import HTTPException, Request

from alert_agent.domain.ingestion.path_rules import PathRuleError
from alert_agent.domain.shared import FatalError
from alert_agent.transport.http.dto.ingest import (
    IngestAcceptedDto,
    IngestRequestDto,
    IngestStatusDto,
)
from alert_agent.transport.http.mapper import (
    accepted_from,
    status_from,
    to_accept_kwargs,
)


def _acceptance(request: Request):
    use_case = getattr(request.app.state, "ingestion_accept", None)
    if use_case is None:
        raise HTTPException(
            status_code=503,
            detail={"code": "NOT_READY", "message": "ingestion stack is not wired yet"},
        )
    return use_case


async def accept(request: Request, dto: IngestRequestDto) -> IngestAcceptedDto:
    use_case = _acceptance(request)
    try:
        job = await use_case.execute(**to_accept_kwargs(dto))
    except PathRuleError as exc:
        raise HTTPException(
            status_code=400,
            detail={"code": "INPUT_INVALID", "message": str(exc)},
        ) from exc
    except FatalError as exc:
        raise HTTPException(
            status_code=400,
            detail={"code": exc.code.value, "message": exc.message},
        ) from exc
    return accepted_from(job)


async def status(doc_id: str, request: Request) -> IngestStatusDto:
    use_case = _acceptance(request)
    job = await use_case.status_of(doc_id)
    if job is None:
        raise HTTPException(
            status_code=404,
            detail={"code": "NOT_FOUND", "message": f"unknown docId {doc_id}"},
        )
    return status_from(job)