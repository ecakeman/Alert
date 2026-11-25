"""Task identity and frozen-boundary value objects (design 3.1.1, data constraints 7.1).

Unknown task types are rejected by the ``TaskType`` literal (7.1.3); generation
is a positive, monotonically increasing integer (7.1.4); the lease token is only
carried through, never generated here (7.1.5); the deadline must carry an
explicit timezone; the version range mirrors Go migration 0003 semantics. All
models are frozen: task identity never mutates after acceptance.
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