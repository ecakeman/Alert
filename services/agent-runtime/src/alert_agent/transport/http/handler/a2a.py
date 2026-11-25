"""A2A server handlers (T22, design 4.5.1, CR-GW-08, CR-LAY-04).

``/.well-known/agent.json`` publishes the fixed three-skill card;
``/a2a/message/send`` maps an A2A Message request onto the SAME application
use cases as the HTTP routes (retrieval -> dual-leg searcher; diagnosis/
archive -> task-dispatch guidance) -- no second business implementation.
message/send requires the internal token (T18 dependency, constant-time
compare); the card is public discovery metadata.
"""

from __future__ import annotations

import json

from fastapi import HTTPException, Request

from alert_agent.infrastructure.a2a.card import build_agent_card


async def agent_card() -> dict:
    """Public AgentCard discovery endpoint."""
    return json.loads(build_agent_card().model_dump_json())


def _extract_skill_and_text(body: dict) -> tuple[str, str]:
    """Minimal A2A Message parsing: metadata.skill picks the skill; text parts
    concatenate into the query."""
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
        # Task skills execute only through Go's SubmitTask dispatch; A2A
        # message/send answers with the authoritative entry-point guidance.
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