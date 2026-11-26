"""AgentCard（T22，design 4.5.1，CR-GW-08）：三个 skill，含 id、描述与
输入/输出示例；发布在 ``/.well-known/agent.json``。
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class AgentSkill(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: str = Field(min_length=1)
    description: str = Field(min_length=1)
    input_example: str = Field(min_length=1)
    output_example: str = Field(min_length=1)


class AgentCard(BaseModel):
    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1)
    description: str = Field(min_length=1)
    protocol: str = "a2a-minimal-v1"
    skills: tuple[AgentSkill, ...] = ()


def build_agent_card() -> AgentCard:
    """固定的三 skill 卡片（诊断 / 归档 / 检索）。"""
    return AgentCard(
        name="alert-agent-runtime",
        description="Alert cognitive runtime: diagnosis, archive and RAG retrieval skills.",
        skills=(
            AgentSkill(
                id="diagnosis",
                description="故障诊断：任务由 Go 控制面经 SubmitTask 派发后执行，产出双产物。",
                input_example="SubmitTask(task_id, episode_id, DIAGNOSIS, generation, lease_token)",
                output_example="INVESTIGATION + DIAGNOSIS_REPORT artifacts",
            ),
            AgentSkill(
                id="archive",
                description="归档索引：从双产物提取故障检索索引并落库。",
                input_example="SubmitTask(task_id, episode_id, ARCHIVE, generation, lease_token)",
                output_example="fault_documents row (READY)",
            ),
            AgentSkill(
                id="retrieval",
                description="知识检索问答：对检索库执行双腿检索并返回命中。",
                input_example='{"query": "gateway 5xx", "topK": 5}',
                output_example="RetrievedChunk list with scores and traceable ids",
            ),
        ),
    )