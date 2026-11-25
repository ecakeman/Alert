"""IngestJob 值模型（design 4.7，CR-ING-02）。

持久化的 job 是摄入状态的唯一权威来源
（QUEUED/PROCESSING/READY/FAILED）；进程内字典绝不持有状态
（CR-ING-02）。``error_msg`` 上限为 500 字符，且绝不携带
凭据或完整路径（7.5.6）；出于同样原因，``file_path`` 始终相对于
摄入根目录。
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