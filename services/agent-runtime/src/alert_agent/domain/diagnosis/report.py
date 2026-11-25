"""DiagnosisReport 值模型（design 3.1.2，7.2.6）。

字段集合严格（在本代码库中 pydantic 默认禁止额外字段），
而列表类型的字段接受松散的标量输入并规范化为
单元素列表（7.2.6，CR-ARCH-05 纪律）；``summary`` 为必填。
``unknowns`` 用于记录预算耗尽与知识/MCP 失败，
而不是编造结论（CR-DIAG-14，CR-DIAG-16）。
"""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator

_LIST_FIELDS = ("observations", "hypotheses", "unknowns", "next_steps")


class DiagnosisReport(BaseModel):
    model_config = ConfigDict(frozen=True)

    summary: str = Field(min_length=1)
    observations: list[str] = Field(default_factory=list)
    hypotheses: list[str] = Field(default_factory=list)
    unknowns: list[str] = Field(default_factory=list)
    next_steps: list[str] = Field(default_factory=list)

    @field_validator(*_LIST_FIELDS, mode="before")
    @classmethod
    def _scalar_to_list(cls, value: Any) -> Any:
        if value is None:
            return []
        if isinstance(value, str):
            return [value]
        return value