"""RAG loop node implementations (T17, design 3.2.2, CR-RAG-05~17).

Eight nodes (rewrite/router/decompose/retrieve/grade/sufficiency/synthesize/
verify) plus the explicit refuse and done nodes. Nodes return incremental
dicts only (CR-REG-02); every trace entry carries real elapsed milliseconds
(CR-REG-12). ``emit`` and ``started_at`` arrive per invocation through the
runnable config -- the graph stays a singleton without emit binding
(CR-RAG-15, CR-REG-11). Grade degradation gives every candidate a default 7
(>= threshold) and sets ``degraded`` so a degraded grade still allows
answering (CR-RAG-11); out-of-range candidate ids are dropped (CR-RAG-09).
"""

from __future__ import annotations

import json
import logging
from typing import Any, Literal

from langgraph.config import RunnableConfig
from pydantic import BaseModel, Field

from alert_agent.application.retrieval.ports import LlmJson, Searcher, Streamer
from alert_agent.application.shared.ports import Clock
from alert_agent.domain.retrieval.budget import RetrievalBudget
from alert_agent.domain.retrieval.citation import Citation
from alert_agent.domain.retrieval.grade import GradeDecision, GradeItem
from alert_agent.domain.retrieval.sse import EVENT_TOKEN, EVENT_TRACE
from alert_agent.domain.retrieval.state import RetrievalState
from alert_agent.domain.retrieval.trace import (
    RewriteTrace,
    SufficiencyDecision,
    TraceNode,
)
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

LOGGER = logging.getLogger(__name__)

_GRADE_FALLBACK_SCORE = 7  # >= threshold 6: degraded grade still answers (CR-RAG-11)
_EXCERPT_CHARS = 200


class RewriteOutput(BaseModel):
    standalone_query: str = Field(min_length=1)


class RouteOutput(BaseModel):
    qtype: Literal["SIMPLE", "COMPLEX", "CHITCHAT"]


class DecomposeOutput(BaseModel):
    sub_questions: list[str] = Field(min_length=1)


class GradeOutput(BaseModel):
    items: list[GradeItem]


class SufficiencyOutput(BaseModel):
    verdict: Literal["ENOUGH", "NEED_MORE"]
    missing: str = ""
    next_query: str = ""


class RetrievalNodes:
    """Node bundle wired into the compiled retrieval graph (singleton-safe:
    all per-invocation state arrives through the config)."""

    def __init__(
        self,
        *,
        searcher: Searcher,
        llm_json: LlmJson,
        streamer: Streamer,
        budget: RetrievalBudget,
        refusal_message: str,
        grade_threshold: int,
        clock: Clock,
    ) -> None:
        self._searcher = searcher
        self._llm_json = llm_json
        self._streamer = streamer
        self._budget = budget
        self._refusal_message = refusal_message
        self._grade_threshold = grade_threshold
        self._clock = clock

    @property
    def clock(self) -> Clock:
        """Public read-only clock for the graph singleton's router wiring."""
        return self._clock

    @property
    def budget(self) -> RetrievalBudget:
        """Public read-only budget for the use-case entry (initial state)."""
        return self._budget

    async def _emit(self, config: RunnableConfig, event: str, data: dict[str, Any]) -> None:
        configurable = (config or {}).get("configurable", {})
        emit = configurable.get("emit")
        if emit is None:
            return
        await emit(event, data)

    async def _trace(
        self, config: RunnableConfig, node: str, status: str, detail: str, started_at_ms: float
    ) -> TraceNode:
        now = self._clock.now()
        ts_ms = (now.timestamp() - started_at_ms) * 1000.0
        entry = TraceNode(node=node, status=status, detail=detail[:800], ts_ms=ts_ms)
        await self._emit(
            config,
            EVENT_TRACE,
            {"node": node, "status": status, "detail": entry.detail, "tsMs": ts_ms},
        )
        return entry

    def _now_ms(self) -> float:
        return self._clock.now().timestamp()

    async def rewrite(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        if not state.history:
            # No history: skip the model entirely (design 3.2.2).
            trace = await self._trace(config, "rewrite", "skipped", "no history", started)
            query = state.question
            return {
                "standalone_query": query,
                "rewrite_trace": RewriteTrace(
                    original_question=state.question, standalone_query=query, ts_ms=trace.ts_ms
                ),
                "trace": [trace],
            }
        history_text = "\n".join(
            f"{turn.role}: {turn.content}" for turn in state.history[-3:]
        )
        output = await self._llm_json.complete_json(
            RewriteOutput,
            system="将用户问题改写为不依赖上下文也可理解的独立检索查询，输出 JSON。",
            user=f"[对话历史]\n{history_text}\n\n[当前问题]\n{state.question}",
        )
        trace = await self._trace(config, "rewrite", "ok", output.standalone_query, started)
        return {
            "standalone_query": output.standalone_query,
            "rewrite_trace": RewriteTrace(
                original_question=state.question,
                standalone_query=output.standalone_query,
                ts_ms=trace.ts_ms,
            ),
            "trace": [trace],
            "llm_calls": state.llm_calls + 1,
        }

    async def router(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        output = await self._llm_json.complete_json(
            RouteOutput,
            system="判断问题类型：SIMPLE（单点事实/故障查询）、COMPLEX（需要拆解的多跳问题）、CHITCHAT（寒暄/与告警运维无关）。输出 JSON。",
            user=state.standalone_query or state.question,
        )
        trace = await self._trace(config, "router", "ok", output.qtype, started)
        return {"qtype": output.qtype, "trace": [trace], "llm_calls": state.llm_calls + 1}

    async def decompose(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        output = await self._llm_json.complete_json(
            DecomposeOutput,
            system="将复杂问题拆解为 2~4 个可独立检索的子问题，输出 JSON 数组。",
            user=state.standalone_query or state.question,
        )
        sub_questions = tuple(dict.fromkeys(q for q in output.sub_questions if q.strip()))
        trace = await self._trace(config, "decompose", "ok", json.dumps(sub_questions, ensure_ascii=False), started)
        return {
            "sub_questions": sub_questions,
            "trace": [trace],
            "llm_calls": state.llm_calls + 1,
        }

    async def retrieve(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        query = state.current_query or state.standalone_query or state.question
        result = await self._searcher.search(
            query=query,
            extra_queries=tuple(q for q in state.sub_questions if q != query),
            vector_top_k=8,
            fusion_top_k=10,
        )
        existing_ids = {chunk.chunk_id for chunk in state.hits}
        new_hits = [chunk for chunk in result.chunks if chunk.chunk_id not in existing_ids]
        zero_new = 0 if new_hits else state.zero_new_hits + 1
        detail = json.dumps(
            {"query": query, "new": len(new_hits), "legs": result.leg_status},
            ensure_ascii=False,
        )
        trace = await self._trace(config, "retrieve", "ok", detail, started)
        return {
            "current_query": query,
            "candidates": result.chunks,
            "hits": new_hits,  # ReduceAppend merges into the deduplicated pool
            "new_hit_count": len(new_hits),
            "zero_new_hits": zero_new,
            "hop": state.hop + 1,
            "trace": [trace],
        }

    async def grade(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        if not state.candidates:
            trace = await self._trace(config, "grade", "skipped", "no candidates", started)
            return {
                "grade_decision": GradeDecision(threshold=self._grade_threshold),
                "trace": [trace],
            }
        try:
            listing = "\n".join(
                f"[{index}] {chunk.content[:300]}" for index, chunk in enumerate(state.candidates)
            )
            output = await self._llm_json.complete_json(
                GradeOutput,
                system=(
                    "对每个候选片段按与问题的相关性打 0~10 分并给出 id，"
                    "输出 JSON。仅返回存在的 id。"
                ),
                user=f"[问题]\n{state.standalone_query or state.question}\n\n[候选片段]\n{listing}",
            )
            # Out-of-range ids are dropped and counted (CR-RAG-09).
            size = len(state.candidates)
            items = tuple(
                item for item in output.items if 0 <= item.id < size
            )
            dropped = len(output.items) - len(items)
            accepted = tuple(item.id for item in items if item.score >= self._grade_threshold)
            decision = GradeDecision(
                items=items,
                threshold=self._grade_threshold,
                accepted_ids=accepted,
                degraded=False,
            )
            trace = await self._trace(
                config,
                "grade",
                "ok",
                f"accepted={len(accepted)} dropped={dropped}",
                started,
            )
            llm_calls = state.llm_calls + 1
        except (RetryableError, FatalError) as exc:
            # CR-RAG-11: degradation stays self-consistent -- default scores
            # stay at/above the threshold so the loop can still answer.
            size = len(state.candidates)
            decision = GradeDecision(
                items=tuple(GradeItem(id=i, score=_GRADE_FALLBACK_SCORE) for i in range(size)),
                threshold=self._grade_threshold,
                accepted_ids=tuple(range(size)),
                degraded=True,
            )
            trace = await self._trace(
                config, "grade", "degraded", type(exc).__name__, started
            )
            LOGGER.warning("grade fallback engaged: %s", type(exc).__name__)
            llm_calls = state.llm_calls + 1
        return {"grade_decision": decision, "trace": [trace], "llm_calls": llm_calls}

    async def sufficiency(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        output = await self._llm_json.complete_json(
            SufficiencyOutput,
            system=(
                "判断已有信息是否足以回答问题。不足以回答时给出 NEED_MORE 与下一跳检索词。输出 JSON。"
            ),
            user=f"[问题]\n{state.standalone_query or state.question}\n\n[已检索]\n无有效命中",
        )
        decision = SufficiencyDecision(
            verdict=output.verdict, missing=output.missing, next_query=output.next_query
        )
        trace = await self._trace(config, "sufficiency", "ok", decision.verdict, started)
        return {"sufficiency": decision, "trace": [trace], "llm_calls": state.llm_calls + 1}

    async def synthesize(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        decision = state.grade_decision
        accepted = (
            [state.candidates[i] for i in decision.accepted_ids if i < len(state.candidates)]
            if decision
            else []
        )
        if not accepted:
            raise FatalError(ErrorCode.INPUT_INVALID, "synthesize reached without accepted chunks")
        context_lines = [
            f"[{position + 1}] (chunk {chunk.chunk_id}) {chunk.content[:600]}"
            for position, chunk in enumerate(accepted)
        ]
        context = "\n".join(context_lines)
        parts: list[str] = []
        async for token in self._streamer.stream(
            system=(
                "你是告警运维知识助手。只依据提供的资料用简体中文回答，"
                "并在引用处标注 [编号]。资料不足时明确说明。"
            ),
            user=f"[问题]\n{state.standalone_query or state.question}\n\n[资料]\n{context}",
        ):
            parts.append(token)
            await self._emit(config, EVENT_TOKEN, {"text": token})
        answer = "".join(parts)
        citations = tuple(
            Citation(
                no=position + 1,
                chunk_id=chunk.chunk_id,
                doc_id=chunk.doc_id,
                section_path=chunk.section_path,
                page=chunk.page,
                text_excerpt=chunk.content[:_EXCERPT_CHARS],
            )
            for position, chunk in enumerate(accepted)
        )
        trace = await self._trace(
            config, "synthesize", "ok", f"chars={len(answer)} citations={len(citations)}", started
        )
        return {
            "answer": answer,
            "citations": citations,
            "trace": [trace],
            "llm_calls": state.llm_calls + 1,
        }

    async def verify(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        known_ids = {chunk.chunk_id for chunk in state.hits} | {
            chunk.chunk_id for chunk in state.candidates
        }
        valid = bool(state.answer) and all(
            citation.chunk_id in known_ids
            and citation.no == position + 1
            for position, citation in enumerate(state.citations)
        )
        verdict = "PASS" if valid else "FAIL"
        trace = await self._trace(config, "verify", verdict, f"citations={len(state.citations)}", started)
        # verify self-counts; routing only reads the counter (CR-RAG-14).
        return {"verified": verdict, "verify_attempts": state.verify_attempts + 1, "trace": [trace]}

    async def refuse(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        trace = await self._trace(config, "refuse", "ok", "fixed refusal, no fabricated answer", started)
        # Fixed wording + empty citations; never model-invented content (3.2.4).
        return {"answer": self._refusal_message, "citations": (), "verified": "SKIPPED", "trace": [trace]}

    async def done(self, state: RetrievalState, config: RunnableConfig) -> dict[str, Any]:
        started = self._now_ms()
        trace = await self._trace(
            config,
            "done",
            "ok",
            f"verified={state.verified} attempts={state.verify_attempts}",
            started,
        )
        return {"trace": [trace]}
