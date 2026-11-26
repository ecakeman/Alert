"""诊断预算模型（design 3.1.3，CR-DIAG-13/14，D10）。

默认值与 D10 的量级保持一致，且可由集中式的
Settings 覆盖（DIAGNOSIS_MAX_TOOL_ROUNDS 族，T02）。任何预算耗尽都必须
导向显式的 ``partial=true`` 结果并附带 unknowns——绝不使用脚本化的
回退结论（CR-DIAG-14）。
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class DiagnosisBudget(BaseModel):
    model_config = ConfigDict(frozen=True)

    max_tool_rounds: int = Field(default=6, ge=1)
    max_llm_calls: int = Field(default=8, ge=1)
    deadline_ratio: float = Field(default=0.8, gt=0, le=1)
    max_no_progress: int = Field(default=2, ge=1)