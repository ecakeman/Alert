"""Ingest 受理与状态查询处理器（T19，CR-LAY-04，CR-REG-52）。

仅做薄处理器：绑定、校验、调用受理用例、映射 DTO、写出响应。规则违规映射
为标准 4xx，并在响应体中携带稳定码；未知文档映射为 404（状态查询以持久化
的 job 为准，CR-ING-06）。
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