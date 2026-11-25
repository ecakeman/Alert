"""归档基础设施：LLM 索引提取与产物正文读取（T14，CR-ARCH-04/05）。

``LlmFaultIndexExtractor`` 是唯一且显式命名的策略（``llm_structured``）：
四字段索引由真实模型经 T11 结构化客户端产出，并通过严格的
``FaultSearchIndex`` 模型校验（含宽松的标量->列表归一化）。此处不存在
确定性回退——模型不可用即归档失败，任务可重新派发（CR-DISC-05）。
``ArtifactFileReader`` 在存储根目录内解析 storage_key，并在 ``resolve()``
之后再次做包含性检查（5.3.4 纪律）。
"""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path

from alert_agent.application.archive.ports import ArchiveExtraction
from alert_agent.domain.archive.index import FaultSearchIndex
from alert_agent.domain.shared import ErrorCode, FatalError
from alert_agent.domain.shared.storage_key import StorageKeyError, parse_storage_key
from alert_agent.infrastructure.llm.client import OpenAiCompatibleLlmClient

LOGGER = logging.getLogger(__name__)

_EXTRACTION_SYSTEM = (
    "你是故障知识归档器。从诊断调查记录与诊断报告中提取故障检索索引。"
    "输出一个 JSON 对象，字段固定为 summary/symptoms/observations/diagnosis："
    "summary 是一句话故障概要；symptoms 是故障表现（用户/系统看到的现象）；"
    "observations 是调查中确认的证据与事实（与 symptoms 语义不同，禁止复制 symptoms 内容）；"
    "diagnosis 是诊断结论列表。列表元素使用简体中文，禁止输出解释文字或代码围栏。"
)

_INVESTIGATION_CHARS = 12000
_REPORT_CHARS = 6000


class LlmFaultIndexExtractor:
    """通过共享的 OpenAI 兼容客户端实现 ``IndexExtractor``。"""

    def __init__(self, client: OpenAiCompatibleLlmClient) -> None:
        self._client = client

    async def extract(
        self, investigation_markdown: str, report_json: str
    ) -> ArchiveExtraction:
        user = (
            "[INVESTIGATION]\n"
            + investigation_markdown[:_INVESTIGATION_CHARS]
            + "\n\n[DIAGNOSIS_REPORT_JSON]\n"
            + report_json[:_REPORT_CHARS]
        )
        index = await self._client.complete_json(
            FaultSearchIndex,
            system=_EXTRACTION_SYSTEM,
            user=user,
        )
        LOGGER.info(
            "fault index extracted (strategy=llm_structured, symptoms=%d, observations=%d)",
            len(index.symptoms),
            len(index.observations),
        )
        return ArchiveExtraction(index=index, strategy="llm_structured")


class ArtifactFileReader:
    """按 storage_key 读取 ARTIFACT_STORAGE_ROOT 内的产物正文。"""

    def __init__(self, root: Path) -> None:
        self._root = root.resolve()

    async def read(self, storage_key: str) -> str:
        try:
            parse_storage_key(storage_key)  # 格式与路径穿越校验
        except StorageKeyError as exc:
            raise FatalError(
                ErrorCode.INPUT_INVALID, f"invalid storage_key: {exc}"
            ) from exc
        relative = storage_key[len("file:") :]
        target = (self._root / relative).resolve()
        if not target.is_relative_to(self._root):
            # resolve 之后的符号链接防御（5.3.4）。
            raise FatalError(
                ErrorCode.INPUT_INVALID, "storage_key escapes the artifact root"
            )
        try:
            return await asyncio.to_thread(target.read_text, "utf-8")
        except OSError as exc:
            raise FatalError(
                ErrorCode.ARTIFACT_MISSING,
                f"artifact body unreadable: {type(exc).__name__}",
            ) from exc