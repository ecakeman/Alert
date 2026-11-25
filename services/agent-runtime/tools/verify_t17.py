"""T17 static verification: graph singleton, routing paths, emit channel (temporary).

The graph is a module-level singleton, so scenarios switch programmable fake
state instead of replacing node instances.
"""

import asyncio
import sys
from datetime import UTC, datetime

sys.path.insert(0, "/home/sancho/workspace/projects/Alert/services/agent-runtime/src")

from alert_agent.application.retrieval.execute import ChatAnswer
from alert_agent.application.retrieval.graph import get_compiled_graph
from alert_agent.application.retrieval.nodes import RetrievalNodes
from alert_agent.domain.retrieval.budget import RetrievalBudget
from alert_agent.domain.retrieval.chat import ChatRequest, ChatTurn
from alert_agent.domain.retrieval.chunk import RetrievalResult, RetrievedChunk
from alert_agent.domain.shared import ErrorCode, FatalError


def chunk(cid: str, content: str) -> RetrievedChunk:
    return RetrievedChunk(
        chunk_id=cid, doc_id="d1", content=content, score=0.9, leg="vector"
    )


class FakeLlmJson:
    """Programmable LlmJson returning the REAL node output models; mutable
    state lets scenarios switch behaviour without swapping instances."""

    def __init__(self) -> None:
        self.qtype = "SIMPLE"
        self.grade_fails = False

    async def complete_json(self, model_cls, *, system, user):
        name = model_cls.__name__
        if name == "RewriteOutput":
            return model_cls(standalone_query="gateway 5xx pooled")
        if name == "RouteOutput":
            return model_cls(qtype=self.qtype)
        if name == "DecomposeOutput":
            return model_cls(sub_questions=["q1", "q2"])
        if name == "GradeOutput":
            if self.grade_fails:
                raise FatalError(ErrorCode.INPUT_INVALID, "grade exploded")
            return model_cls(
                items=[{"id": 0, "score": 9}, {"id": 1, "score": 3}, {"id": 99, "score": 5}]
            )
        if name == "SufficiencyOutput":
            return model_cls(verdict="ENOUGH")
        raise AssertionError(f"unexpected model: {name}")


class FakeSearcher:
    def __init__(self) -> None:
        self.chunks: list[RetrievedChunk] = []

    async def search(self, *, query, extra_queries=(), doc_ids=None, vector_top_k=8, fusion_top_k=10):
        return RetrievalResult(
            chunks=tuple(self.chunks), leg_status={"vector": "ok", "bm25": "ok"}
        )


class FakeStreamer:
    async def stream(self, *, system, user):
        for token in ["网关 ", "5xx 是 ", "连接池耗尽 [1]。"]:
            yield token


class FakeClock:
    def __init__(self) -> None:
        self._now = datetime.now(UTC)

    def now(self):
        return self._now


async def main() -> None:
    llm = FakeLlmJson()
    searcher = FakeSearcher()
    searcher.chunks = [chunk("c1", "连接池耗尽证据"), chunk("c2", "无关内容")]

    nodes = RetrievalNodes(
        searcher=searcher,
        llm_json=llm,
        streamer=FakeStreamer(),
        budget=RetrievalBudget(),
        refusal_message="依据现有资料无法回答，请换问法或补充资料。",
        grade_threshold=6,
        clock=FakeClock(),
    )

    # --- singleton: two calls return the same compiled graph ---
    graph_a = await get_compiled_graph(lambda: nodes)
    graph_b = await get_compiled_graph(lambda: nodes)
    assert graph_a is graph_b
    node_names = set(graph_a.get_graph().nodes)
    assert {
        "rewrite", "router", "decompose", "retrieve", "grade",
        "sufficiency", "synthesize", "verify", "refuse", "done",
    } <= node_names

    case = ChatAnswer(nodes=nodes, clock=FakeClock())
    request = ChatRequest(
        session_id="s1",
        message="网关为什么 5xx",
        history=(ChatTurn(role="user", content="之前问过超时"),),
    )

    events: list[tuple[str, dict]] = []

    async def emit(event: str, data: dict) -> None:
        events.append((event, data))

    # --- SIMPLE happy path ---
    final = await case.chat(request, emit)
    assert final.answer == "网关 5xx 是 连接池耗尽 [1]。", final.answer
    assert final.citations and final.citations[0].no == 1
    assert final.verified == "PASS"
    assert final.qtype == "SIMPLE"
    assert all(node.ts_ms >= 0 for node in final.trace)
    kinds = [event for event, _ in events]
    assert "trace" in kinds and "token" in kinds
    assert all(item.id in (0, 1) for item in final.grade_decision.items)
    assert final.grade_decision.accepted_ids == (0,)  # id=99 dropped, score 3 rejected

    # --- grade degradation (CR-RAG-11): fallback all-7, degraded=True ---
    llm.grade_fails = True
    final = await case.chat(request)
    assert final.grade_decision is not None
    assert final.grade_decision.degraded is True
    assert all(item.score == 7 for item in final.grade_decision.items)
    assert final.answer
    llm.grade_fails = False

    # --- CHITCHAT: router routes to refuse with the fixed message ---
    llm.qtype = "CHITCHAT"
    final = await case.chat(request)
    assert final.answer == "依据现有资料无法回答，请换问法或补充资料。"
    assert final.citations == () and final.verified == "SKIPPED"
    llm.qtype = "SIMPLE"

    # --- empty index + SIMPLE -> refuse (distinguishable empty) ---
    searcher.chunks = []
    final = await case.chat(request)
    assert final.answer.startswith("依据现有资料")
    assert final.citations == ()

    print("T17 static verification OK")


asyncio.run(main())