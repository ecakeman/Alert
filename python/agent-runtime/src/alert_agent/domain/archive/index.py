"""归档领域：故障搜索索引模型与幂等的 fd 标识（M6）。

``FaultSearchIndex`` 仅携带四个可搜索字段（CR-ARCH-03）；
列表字段接受松散标量并规范化为列表，同时字段集合
保持严格（CR-ARCH-05）；symptoms 与 observations 必须是真正
不同的语义映射，绝不重复内容（CR-ARCH-03）。``fd_id``
由 episode 确定性推导，因此重新归档可复现该值
（CR-ARCH-06 幂等键）。
"""

from __future__ import annotations

import hashlib
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

_LIST_FIELDS = ("symptoms", "observations", "diagnosis")


def fd_id_of(episode_id: str) -> str:
    """稳定的故障文档 id：由 episode id 确定性推导（CR-ARCH-06）。"""
    digest = hashlib.sha256(episode_id.encode("utf-8")).hexdigest()[:24]
    return f"fd_{digest}"


def content_hash_of(index: FaultSearchIndex) -> str:
    """对规范化的四字段 JSON 计算 sha-256（注册时携带该值）。"""
    payload = index.model_dump_json()
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


class FaultSearchIndex(BaseModel):
    model_config = ConfigDict(frozen=True)

    summary: str = Field(min_length=1)
    symptoms: list[str] = Field(default_factory=list)
    observations: list[str] = Field(default_factory=list)
    diagnosis: list[str] = Field(default_factory=list)

    @field_validator(*_LIST_FIELDS, mode="before")
    @classmethod
    def _scalar_to_list(cls, value: Any) -> Any:
        if value is None:
            return []
        if isinstance(value, str):
            return [value]
        return value

    @field_validator(*_LIST_FIELDS)
    @classmethod
    def _no_empty_items(cls, value: list[str]) -> list[str]:
        cleaned = [item.strip() for item in value if item.strip()]
        return cleaned

    @model_validator(mode="after")
    def _symptoms_differ_from_observations(self) -> FaultSearchIndex:
        # CR-ARCH-03：两个列表必须映射真正不同的语义。
        if self.symptoms == self.observations:
            raise ValueError(
                "symptoms and observations must not be identical (CR-ARCH-03)"
            )
        if not self.symptoms or not self.observations:
            raise ValueError("symptoms and observations must both be non-empty")
        return self


class FaultDocument(BaseModel):
    """以业务字段表示的故障文档行值对象（不含向量）。"""

    model_config = ConfigDict(frozen=True)

    fd_id: str = Field(min_length=1)
    episode_id: str = Field(min_length=1)
    service_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    generation: int = Field(gt=0)
    index: FaultSearchIndex
    content_hash: str = Field(min_length=1)