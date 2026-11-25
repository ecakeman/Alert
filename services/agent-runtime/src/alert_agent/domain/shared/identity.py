"""任务身份与冻结边界值对象（design 3.1.1，数据约束 7.1）。

未知的任务类型会被 ``TaskType`` 字面量拒绝（7.1.3）；generation
为正且单调递增的整数（7.1.4）；lease token 仅作透传，
绝不在此生成（7.1.5）；deadline 必须携带
显式时区；版本范围与 Go migration 0003 语义一致。所有
模型均为冻结：任务身份在受理之后绝不变更。
"""

from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

TaskType = Literal["DIAGNOSIS", "ARCHIVE"]


class TaskIdentity(BaseModel):
    model_config = ConfigDict(frozen=True)

    task_id: str = Field(min_length=1)
    episode_id: str = Field(min_length=1)
    task_type: TaskType
    generation: int = Field(gt=0)
    lease_token: str = Field(min_length=1)


class FrozenBoundary(BaseModel):
    model_config = ConfigDict(frozen=True)

    input_version_from: int = Field(gt=0)
    input_version_to: int = Field(gt=0)
    monitor_watermark: int = Field(ge=0)
    app_event_watermark: int = Field(ge=0)
    deadline_at: datetime

    @field_validator("deadline_at")
    @classmethod
    def _require_timezone(cls, value: datetime) -> datetime:
        if value.tzinfo is None or value.tzinfo.utcoffset(value) is None:
            raise ValueError("deadline_at must carry an explicit timezone")
        return value

    @model_validator(mode="after")
    def _check_version_range(self) -> FrozenBoundary:
        if self.input_version_from > self.input_version_to:
            raise ValueError("input_version_from must be <= input_version_to")
        return self