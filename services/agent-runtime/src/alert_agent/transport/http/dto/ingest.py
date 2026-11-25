"""Ingest 线上 DTO：按照 RAG 协议风格采用 camelCase（CR-LAY-10）。"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class IngestRequestDto(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    doc_id: str = Field(min_length=1, alias="docId")
    file_name: str = Field(min_length=1, alias="fileName")
    title: str = Field(min_length=1)
    callback_url: str = Field(min_length=1, alias="callbackUrl")
    file_path: str = Field(min_length=1, alias="filePath")


class IngestAcceptedDto(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    doc_id: str = Field(min_length=1, alias="docId")
    status: str


class IngestStatusDto(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    doc_id: str = Field(min_length=1, alias="docId")
    status: str
    chunk_count: int | None = Field(default=None, alias="chunkCount")
    error_msg: str = Field(default="", alias="errorMsg")