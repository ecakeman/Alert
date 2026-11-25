"""DiagnosisReport value model (design 3.1.2, 7.2.6).

Field set is strict (extra fields forbidden by pydantic default in this code
base) while list-valued fields accept a loose scalar input and normalize it to
a one-element list (7.2.6, CR-ARCH-05 discipline); ``summary`` is mandatory.
``unknowns`` is where budget exhaustion and knowledge/MCP failures are
recorded instead of fabricated conclusions (CR-DIAG-14, CR-DIAG-16).
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