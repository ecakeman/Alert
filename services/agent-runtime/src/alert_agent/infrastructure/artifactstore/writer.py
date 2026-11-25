"""Filesystem implementation of the ArtifactWriter port (design 4.6, CR-GW-09/09a).

Content lives only in files (D5): the writer resolves the storage_key through
the domain rules, re-checks containment after ``resolve()`` (symlink defence,
5.3.4) and writes atomically with a temp file + ``os.replace``. The sha-256
digest of the written bytes is returned so the cognitive registration and the
RegisterTaskArtifact callback share one hash computation (CR-LAY-10) and the
registered hash always matches the file body (CR-GW-09a).
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
    """ArtifactWriter port implementation on ARTIFACT_STORAGE_ROOT (design 4.6)."""

    def __init__(self, root: Path) -> None:
        self._root = root.resolve()

    @property
    def root(self) -> Path:
        return self._root

    async def write(self, storage_key: str, content: bytes) -> str:
        ref = parse_storage_key(storage_key)  # format + escape validation (design 4.6, 5.3.4)
        relative = storage_key[len("file:") :]
        target = (self._root / relative).resolve()
        if not target.is_relative_to(self._root):
            # parse_storage_key already rejects traversal; this re-checks
            # symlink-based escapes after resolve() (5.3.4).
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