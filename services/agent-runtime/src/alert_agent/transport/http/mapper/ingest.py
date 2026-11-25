"""Ingest DTO <-> domain mapping: the single conversion point (CR-LAY-10)."""

from __future__ import annotations

from alert_agent.domain.ingestion.job import IngestJob
from alert_agent.transport.http.dto.ingest import (
    IngestAcceptedDto,
    IngestRequestDto,
    IngestStatusDto,
)


def to_accept_kwargs(dto: IngestRequestDto) -> dict:
    return {
        "doc_id": dto.doc_id,
        "file_name": dto.file_name,
        "title": dto.title,
        "callback_url": dto.callback_url,
        "file_path": dto.file_path,
    }


def accepted_from(job: IngestJob) -> IngestAcceptedDto:
    return IngestAcceptedDto(docId=job.doc_id, status=job.status)


def status_from(job: IngestJob) -> IngestStatusDto:
    return IngestStatusDto(
        docId=job.doc_id,
        status=job.status,
        chunkCount=job.chunk_count,
        errorMsg=job.error_msg,
    )