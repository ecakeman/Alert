"""Diagnosis budget model (design 3.1.3, CR-DIAG-13/14, D10).

Defaults mirror the D10 magnitude and stay overridable from the centralized
Settings (DIAGNOSIS_MAX_TOOL_ROUNDS family, T02). Exhausting any budget must
lead to an explicit ``partial=true`` outcome with unknowns -- never a scripted
fallback conclusion (CR-DIAG-14).
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class DiagnosisBudget(BaseModel):
    model_config = ConfigDict(frozen=True)

    max_tool_rounds: int = Field(default=6, ge=1)
    max_llm_calls: int = Field(default=8, ge=1)
    deadline_ratio: float = Field(default=0.8, gt=0, le=1)
    max_no_progress: int = Field(default=2, ge=1)