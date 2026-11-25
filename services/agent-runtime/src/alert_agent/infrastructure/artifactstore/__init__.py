"""Artifact file store: atomic writes under the storage root (design 4.6, D5)."""

from __future__ import annotations

from alert_agent.infrastructure.artifactstore.writer import FileSystemArtifactWriter

__all__ = ["FileSystemArtifactWriter"]