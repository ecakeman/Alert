"""双产物写入/注册编排（design 4.6，CR-DIAG-18~20）。

INVESTIGATION 是由收集到的证据组装而成的确定性 Markdown，每个
task+generation 仅有一个轮次标记（7.2.7）；DIAGNOSIS_REPORT 是结构化的
模型 JSON。内容只存在于文件中（D5）：两个产物都经由
``build_storage_key`` -> ``ArtifactWriter.write``（sha-256）->
``ArtifactStore.register``（版本单调的乐观锁，CR-DIAG-19）->
``RegisterTaskArtifact`` 上报至 Go（CR-DIAG-20）。注册被拒绝则任务失败——
任务绝不能在没有注册的情况下声称成功（CR-ARCH-09 纪律同样适用于
诊断产物）。
"""

from __future__ import annotations

from typing import Any

from alert_agent.application.shared.ports import ArtifactWriter
from alert_agent.application.task.ports import (
    ArtifactCallback,
    ArtifactRegistration,
    ArtifactStore,
    TaskGateway,
)
from alert_agent.domain.diagnosis.state import DiagnosisState
from alert_agent.domain.shared import ErrorCode, FatalError
from alert_agent.domain.shared.storage_key import build_storage_key, content_type_of


def render_investigation_markdown(state: DiagnosisState) -> str:
    """确定性调查记录；由于每次 WRITE 都会创建新的版本文件，轮次标记在
    每个 task+generation 中恰好出现一次。"""
    lines: list[str] = [
        f"# Investigation - episode {state.identity.episode_id}",
        "",
        f"- task: `{state.identity.task_id}` (generation {state.identity.generation})",
        f"- service: `{state.service_id}` / {state.service_name}",
        f"- frozen input versions: {state.boundary.input_version_from}..{state.boundary.input_version_to}",
        f"- watermarks: monitor={state.boundary.monitor_watermark} app_event={state.boundary.app_event_watermark}",
        "",
        "## Timeline",
    ]
    if state.timeline_records:
        for record in state.timeline_records:
            lines.append(
                f"- [{record.record_type}] {record.record_id} @ {record.event_time.isoformat()}"
                f" (source: {record.source})"
            )
    else:
        lines.append("- 无时间轴记录")
    lines += ["", "## Historical fault hits"]
    if state.search_hits:
        for hit in state.search_hits:
            lines.append(f"- {hit.fd_id} ({hit.service_id}) score={hit.score:.3f}: {hit.summary}")
    else:
        lines.append("- 无历史故障命中")
    lines += ["", "## Knowledge"]
    if state.knowledge.called:
        if state.knowledge.error:
            lines.append(f"- 调用失败: {state.knowledge.error}")
        else:
            for knowledge_hit in state.knowledge.results:
                lines.append(f"- {knowledge_hit.hit_id}: {knowledge_hit.title}")
    else:
        lines.append("- 未调用知识智能体")
    lines += ["", "## MCP tool calls"]
    if state.mcp_results:
        for result in state.mcp_results:
            status = "error" if result.is_error else "ok"
            lines.append(f"- {result.name} ({status}), {len(result.content)} content block(s)")
    else:
        lines.append("- 无 MCP 工具调用")
    lines += ["", "## Executed actions"]
    for executed in state.executed_actions:
        lines.append(
            f"- round {executed.round}: {executed.type} ok={executed.ok} - {executed.brief}"
        )
    lines += ["", f"<!-- round-marker task={state.identity.task_id} generation={state.identity.generation} -->"]
    return "\n".join(lines) + "\n"


def artifact_source_refs(state: DiagnosisState) -> tuple[str, ...]:
    """由 RegisterTaskArtifact 携带的可追溯引用（fd id、知识命中、
    MCP 工具）；未消费任何内容时为空。"""
    refs = [hit.fd_id for hit in state.search_hits]
    refs += [hit.hit_id for hit in state.knowledge.results]
    refs += [result.name for result in state.mcp_results]
    return tuple(dict.fromkeys(refs))


class DiagnosisArtifactOrchestrator:
    """注入循环节点的 load_existing_versions / write 回调。"""

    def __init__(
        self,
        *,
        store: ArtifactStore,
        writer: ArtifactWriter,
        gateway: TaskGateway,
    ) -> None:
        self._store = store
        self._writer = writer
        self._gateway = gateway

    async def load_existing_versions(self, state: DiagnosisState) -> dict[str, Any]:
        """LOAD 数据：每种产物类型的最高已注册版本；缺失时为 None，
        使重新派发的 generation 可以复用/幂等续写（Q7）。"""
        episode_id = state.identity.episode_id
        investigation = await self._store.latest_version(episode_id, "INVESTIGATION")
        report = await self._store.latest_version(episode_id, "DIAGNOSIS_REPORT")
        return {
            "investigation_version": investigation if investigation > 0 else None,
            "report_version": report if report > 0 else None,
        }

    async def write(self, state: DiagnosisState) -> dict[str, Any]:
        """WRITE：持久化 + 注册 + 回调两个产物；任何拒绝都以领域错误的
        形式向上传播（任务失败，绝不伪成功）。"""
        if state.report is None:
            raise FatalError(
                ErrorCode.ARTIFACT_MISSING,
                "WRITE reached without a diagnosis report (finish action must provide one)",
            )
        generation = state.identity.generation
        episode_id = state.identity.episode_id
        source_refs = artifact_source_refs(state)

        investigation_version = (state.investigation_version or 0) + 1
        investigation_key = build_storage_key(
            episode_id, "INVESTIGATION", generation, investigation_version
        )
        investigation_bytes = render_investigation_markdown(state).encode("utf-8")
        investigation_hash = await self._writer.write(
            investigation_key, investigation_bytes
        )
        await self._register_and_callback(
            state,
            artifact_type="INVESTIGATION",
            version=investigation_version,
            content_hash=investigation_hash,
            storage_key=investigation_key,
            source_refs=source_refs,
        )

        report_version = (state.report_version or 0) + 1
        report_key = build_storage_key(
            episode_id, "DIAGNOSIS_REPORT", generation, report_version
        )
        report_bytes = (state.report.model_dump_json(indent=2) + "\n").encode("utf-8")
        report_hash = await self._writer.write(report_key, report_bytes)
        await self._register_and_callback(
            state,
            artifact_type="DIAGNOSIS_REPORT",
            version=report_version,
            content_hash=report_hash,
            storage_key=report_key,
            source_refs=source_refs,
        )
        return {
            "investigation_version": investigation_version,
            "report_version": report_version,
        }

    async def _register_and_callback(
        self,
        state: DiagnosisState,
        *,
        artifact_type: str,
        version: int,
        content_hash: str,
        storage_key: str,
        source_refs: tuple[str, ...],
    ) -> None:
        registration = ArtifactRegistration(
            task_id=state.identity.task_id,
            generation=state.identity.generation,
            episode_id=state.identity.episode_id,
            artifact_type=artifact_type,
            artifact_version=version,
            content_hash=content_hash,
            content_type=content_type_of(artifact_type),
            storage_key=storage_key,
        )
        record = await self._store.register(registration)
        await self._gateway.register_artifact(
            ArtifactCallback(
                identity=state.identity,
                artifact_type=artifact_type,
                artifact_version=record.artifact_version,
                content_hash=record.content_hash,
                content_type=record.content_type,
                storage_key=record.storage_key,
                source_refs=source_refs,
            )
        )