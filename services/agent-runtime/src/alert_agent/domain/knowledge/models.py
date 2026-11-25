"""Knowledge invocation domain models (design 4.5.2, CR-KNOW-01~03).

``InvocationV1`` is the only way a knowledge request is built (CR-KNOW-01);
camelCase wire aliases are declared once here and serialization later uses
``model_dump(by_alias=True)`` so no hand-written camelCase payload strings
appear elsewhere (CR-REG-30). ``knownFacts`` is JSON-serialized with
``ensure_ascii=False`` downstream (CR-KNOW-02).
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field


class InvocationConstraints(BaseModel):
    """Explicit topK / requireCitations expression (CR-KNOW-03)."""

    model_config = ConfigDict(frozen=True, populate_by_name=True)

    top_k: int = Field(default=8, ge=1, le=20, alias="topK")
    require_citations: bool = Field(default=True, alias="requireCitations")


class InvocationContext(BaseModel):
    """Caller context attached to one invocation; ``knownFacts`` entries are
    plain strings so downstream serialization stays deterministic."""

    model_config = ConfigDict(frozen=True, populate_by_name=True)

    known_facts: tuple[str, ...] = Field(default=(), alias="knownFacts")
    episode_id: str | None = Field(default=None, alias="episodeId")
    service_id: str | None = Field(default=None, alias="serviceId")


class InvocationV1(BaseModel):
    model_config = ConfigDict(frozen=True, populate_by_name=True)

    schema_version: Literal["v1"] = Field(default="v1", alias="schemaVersion")
    invocation_id: str = Field(min_length=1, alias="invocationId")
    skill: str = Field(min_length=1)
    knowledge_space_ids: list[str] = Field(
        default_factory=list, alias="knowledgeSpaceIds"
    )
    query: str = Field(min_length=1)
    context: InvocationContext = Field(default_factory=InvocationContext)
    constraints: InvocationConstraints = Field(default_factory=InvocationConstraints)


class KnowledgeHit(BaseModel):
    """One knowledge result; must carry a referenceable id and content
    (CR-DIAG-17, CR-KNOW-07) -- malformed hits fail validation instead of
    raising deep AttributeErrors."""

    model_config = ConfigDict(frozen=True, populate_by_name=True)

    hit_id: str = Field(min_length=1, alias="hitId")
    title: str = Field(min_length=1)
    content: str = Field(min_length=1)
    source: str | None = Field(default=None, alias="source")
    score: float | None = Field(default=None, alias="score")