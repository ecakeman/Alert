"""知识调用领域模型（design 4.5.2，CR-KNOW-01~03）。

``InvocationV1`` 是构建知识请求的唯一方式（CR-KNOW-01）；
camelCase 线上别名仅在此声明一次，后续序列化使用
``model_dump(by_alias=True)``，因此其他地方不会出现手写的
camelCase 负载字符串（CR-REG-30）。``knownFacts`` 在下游以
``ensure_ascii=False`` 进行 JSON 序列化（CR-KNOW-02）。
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field


class InvocationConstraints(BaseModel):
    """显式的 topK / requireCitations 表达（CR-KNOW-03）。"""

    model_config = ConfigDict(frozen=True, populate_by_name=True)

    top_k: int = Field(default=8, ge=1, le=20, alias="topK")
    require_citations: bool = Field(default=True, alias="requireCitations")


class InvocationContext(BaseModel):
    """附着在一次调用上的调用方上下文；``knownFacts`` 条目为
    纯字符串，使下游序列化保持确定性。"""

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
    """单条知识结果；必须携带可引用的 id 与内容
    （CR-DIAG-17，CR-KNOW-07）——格式错误的命中在
    校验阶段即失败，而不是在深处抛出 AttributeError。"""

    model_config = ConfigDict(frozen=True, populate_by_name=True)

    hit_id: str = Field(min_length=1, alias="hitId")
    title: str = Field(min_length=1)
    content: str = Field(min_length=1)
    source: str | None = Field(default=None, alias="source")
    score: float | None = Field(default=None, alias="score")