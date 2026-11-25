"""产物文件存储：存储根目录下的原子写入（design 4.6，D5）。"""

from __future__ import annotations

from alert_agent.infrastructure.artifactstore.writer import FileSystemArtifactWriter

__all__ = ["FileSystemArtifactWriter"]