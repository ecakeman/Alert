"""A2A 服务端处理器（T22，design 4.5.1，CR-GW-08，CR-LAY-04）。

``/.well-known/agent.json`` 发布固定的三技能卡片；
``/a2a/message/send`` 将 A2A Message 请求映射到与 HTTP 路由完全相同的
应用用例（检索 -> 双路检索器；诊断/归档 -> 任务分发指引）——不实现第二套
业务逻辑。message/send 需要内部令牌（T18 依赖，恒定时间比较）；卡片是公开
的发现元数据。
"""

from __future__ import annotations

import json

from fastapi import HTTPException, Request

from alert_agent.infrastructure.a2a.card import build_agent_card


async def agent_card() -> dict:
    """公开的 AgentCard 发现端点。"""
    return json.loads(build_agent_card().model_dump_json())


def _extract_skill_and_text(body: dict) -> tuple[str, str]:
    """最小化的 A2A Message 解析：metadata.skill 选择技能；text 部分
    拼接为查询。"""
    message = body.get("message") or {}
    metadata = message.get("metadata") or {}
    skill = str(metadata.get("skill", "retrieval"))
    texts = [
        str(part.get("text", ""))
        for part in message.get("parts", [])
        if isinstance(part, dict) and part.get("kind") == "text"
    ]
    return skill, " ".join(text for text in texts if text.strip())


async def message_send(request: Request) -> dict:
    searcher = getattr(request.app.state, "retrieval_searcher", None)
    body = await request.json()
    skill, text = _extract_skill_and_text(body)
    if skill == "retrieval":
        if searcher is None:
            raise HTTPException(
                status_code=503,
                detail={"code": "NOT_READY", "message": "retrieval stack is not wired yet"},
            )
        if not text.strip():
            raise HTTPException(
                status_code=400,
                detail={"code": "INPUT_INVALID", "message": "message parts carry no query text"},
            )
        result = await searcher.search(query=text, vector_top_k=8, fusion_top_k=10)
        hits = [
            {
                "kind": "data",
                "data": {
                    "chunkId": chunk.chunk_id,
                    "docId": chunk.doc_id,
                    "score": chunk.score,
                    "leg": chunk.leg,
                },
            }
            for chunk in result.chunks
        ]
        return {
            "role": "assistant",
            "parts": [
                {"kind": "text", "text": f"retrieved {len(result.chunks)} chunks"},
                {"kind": "data", "data": {"chunks": hits, "legStatus": result.leg_status}},
            ],
            "metadata": {"skill": "retrieval"},
        }
    if skill in ("diagnosis", "archive"):
        # 任务类技能只能通过 Go 的 SubmitTask 分发执行；A2A
        # message/send 以权威的入口指引作为响应。
        return {
            "role": "assistant",
            "parts": [
                {
                    "kind": "text",
                    "text": (
                        f"The {skill} skill is task-driven: dispatch via "
                        "alertflow.agent.v1.AgentTaskService.SubmitTask; see the "
                        "AgentCard skill examples."
                    ),
                }
            ],
            "metadata": {"skill": skill},
        }
    raise HTTPException(
        status_code=400,
        detail={"code": "INPUT_INVALID", "message": f"unknown skill {skill}"},
    )