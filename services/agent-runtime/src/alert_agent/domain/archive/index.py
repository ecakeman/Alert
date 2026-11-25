"""Archive domain: fault search index models and idempotent fd identity (M6).

``FaultSearchIndex`` carries exactly the four searchable fields (CR-ARCH-03);
list fields accept loose scalars and normalize to lists while the field set
stays strict (CR-ARCH-05); symptoms and observations must be genuinely
different semantic mappings, never duplicate content (CR-ARCH-03). ``fd_id``
derives deterministically from the episode so re-archiving reproduces it
(CR-ARCH-06 idempotency key).
"""

from __future__ import annotations

import hashlib
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

_LIST_FIELDS = ("symptoms", "observations", "diagnosis")


def fd_id_of(episode_id: str) -> str:
    """Stable fault-document id: deterministic from the episode id (CR-ARCH-06)."""
    digest = hashlib.sha256(episode_id.encode("utf-8")).hexdigest()[:24]
    return f"fd_{digest}"


def content_hash_of(index: FaultSearchIndex) -> str:
    """sha-256 over the canonical four-field JSON (registration carries it)."""
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
        # CR-ARCH-03: the two lists must map genuinely different semantics.
        if self.symptoms == self.observations:
            raise ValueError(
                "symptoms and observations must not be identical (CR-ARCH-03)"
            )
        if not self.symptoms or not self.observations:
            raise ValueError("symptoms and observations must both be non-empty")
        return self


class FaultDocument(BaseModel):
    """Fault-document row value object as business fields (no embedding)."""

    model_config = ConfigDict(frozen=True)

    fd_id: str = Field(min_length=1)
    episode_id: str = Field(min_length=1)
    service_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    generation: int = Field(gt=0)
    index: FaultSearchIndex
    content_hash: str = Field(min_length=1)