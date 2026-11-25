"""ArtifactWriter 端口的文件系统实现（design 4.6，CR-GW-09/09a）。

内容只存在于文件中（D5）：写入器经领域规则解析 storage_key，在
``resolve()`` 之后复查包含性（符号链接防御，5.3.4），并以临时文件 +
``os.replace`` 原子写入。返回写入字节的 sha-256 摘要，使认知注册与
RegisterTaskArtifact 回调共享同一次哈希计算（CR-LAY-10），且注册的哈希
始终与文件正文一致（CR-GW-09a）。
"""

from __future__ import annotations

import asyncio
import hashlib
import logging
import os
from pathlib import Path
from uuid import uuid4

from alert_agent.domain.shared import ErrorCode, FatalError
from alert_agent.domain.shared.storage_key import parse_storage_key

LOGGER = logging.getLogger(__name__)


class FileSystemArtifactWriter:
    """ARTIFACT_STORAGE_ROOT 上的 ArtifactWriter 端口实现（design 4.6）。"""

    def __init__(self, root: Path) -> None:
        self._root = root.resolve()

    @property
    def root(self) -> Path:
        return self._root

    async def write(self, storage_key: str, content: bytes) -> str:
        ref = parse_storage_key(storage_key)  # 格式与越界校验（design 4.6，5.3.4）
        relative = storage_key[len("file:") :]
        target = (self._root / relative).resolve()
        if not target.is_relative_to(self._root):
            # parse_storage_key 已拒绝路径穿越；此处复查 resolve() 之后
            # 基于符号链接的越界（5.3.4）。
            raise FatalError(ErrorCode.INPUT_INVALID, "storage_key escapes the artifact root")
        digest = hashlib.sha256(content).hexdigest()
        await asyncio.to_thread(self._atomic_write, target, content)
        LOGGER.info(
            "artifact written episode=%s type=%s version=%d bytes=%d",
            ref.episode_id,
            ref.artifact_type,
            ref.version,
            len(content),
        )
        return digest

    def _atomic_write(self, target: Path, content: bytes) -> None:
        target.parent.mkdir(parents=True, exist_ok=True)
        tmp = target.with_name(f".{target.name}.{uuid4().hex}.tmp")
        try:
            tmp.write_bytes(content)
            os.replace(tmp, target)
        except OSError:
            tmp.unlink(missing_ok=True)
            raise