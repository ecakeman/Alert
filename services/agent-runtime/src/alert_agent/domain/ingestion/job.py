"""IngestJob value model (design 4.7, CR-ING-02).

The persisted job is the single authority for ingestion status
(QUEUED/PROCESSING/READY/FAILED); in-process dicts never hold state
(CR-ING-02). ``error_msg`` is capped at 500 chars and never carries
credentials or full paths (7.5.6); ``file_path`` stays relative to the
ingest root for the same reason.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

IngestStatus = Literal["QUEUED", "PROCESSING", "READY", "FAILED"]

_ERROR_MAX = 500


class IngestJob(BaseModel):
    model_config = ConfigDict(frozen=True)

    doc_id: str = Field(min_length=1)
    status: IngestStatus = "QUEUED"
    chunk_count: int | None = Field(default=None, ge=0)
    error_msg: str = Field(default="", max_length=_ERROR_MAX)
    callback_url: str = Field(min_length=1)
    attempt: int = Field(default=0, ge=0)
    file_path: str = Field(min_length=1)