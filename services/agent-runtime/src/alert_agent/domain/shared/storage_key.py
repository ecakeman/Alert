"""storage_key addressing rules as pure functions (design 4.6, D5, CR-GW-09a).

Format (verified against Go ``internal/infrastructure/artifacts/reader.go``):
``file:{relative-path}`` with forward slashes. The Go reader strips the
``file:`` prefix, cleans the relative path and rejects absolute paths,
``..`` segments and anything escaping the reader root; keys generated here
stay relative and slash-separated so they remain readable by that reader
(when its root is pointed at the shared storage directory, design R4).

Keys embed episode, artifact type, generation and version --
``file:episodes/{episode_id}/diagnosis/g{generation}_{stem}_v{version}{suffix}``
-- so an older generation can never overwrite a newer one's files
(CR-GW-09a, CR-REG-46). No IO in this module (CR-LAY-03).
"""

from __future__ import annotations

import re

from pydantic import BaseModel, ConfigDict, Field

_FILE_PREFIX = "file:"
_SEGMENT = "episodes"
_STEM_BY_TYPE: dict[str, tuple[str, str, str, str]] = {
    # artifact_type -> (path segment, file stem, suffix, content_type)
    "INVESTIGATION": ("diagnosis", "investigation", ".md", "text/markdown"),
    "DIAGNOSIS_REPORT": ("diagnosis", "diagnosis_report", ".json", "application/json"),
}
_FILENAME_PATTERN = re.compile(
    r"^g(?P<generation>\d+)_(?P<stem>investigation|diagnosis_report)"
    r"_v(?P<version>\d+)\.(?P<suffix>md|json)$"
)
_STEM_TO_TYPE = {stem: artifact_type for artifact_type, spec in _STEM_BY_TYPE.items() for stem in (spec[1],)}


class StorageKeyError(ValueError):
    """A storage_key failed the addressing rules (stable semantics, 6.1.3.4)."""


class StorageKeyRef(BaseModel):
    """Parsed storage_key identity (design 4.6)."""

    model_config = ConfigDict(frozen=True)

    episode_id: str = Field(min_length=1)
    artifact_type: str = Field(min_length=1)
    generation: int = Field(gt=0)
    version: int = Field(gt=0)


def content_type_of(artifact_type: str) -> str:
    """The content_type registered alongside the artifact (D5)."""
    spec = _STEM_BY_TYPE.get(artifact_type)
    if spec is None:
        raise StorageKeyError(f"unknown artifact_type: {artifact_type}")
    return spec[3]


def validate_episode_id(value: str) -> str:
    """Reject path-hostile episode ids before they enter a storage_key.

    The Go reader's containment checks stay authoritative; this is the
    generation-side first line of defence (5.3.4 discipline).
    """
    if not value or value in {".", ".."} or value != value.strip():
        raise StorageKeyError(f"episode_id is not a usable single path segment: {value!r}")
    for forbidden in ("/", "\\", "\x00", "\n", "\r"):
        if forbidden in value:
            raise StorageKeyError(f"episode_id must not contain {forbidden!r}")
    return value


def build_storage_key(episode_id: str, artifact_type: str, generation: int, version: int) -> str:
    """Build the storage_key for one artifact version (pure, no IO)."""
    spec = _STEM_BY_TYPE.get(artifact_type)
    if spec is None:
        raise StorageKeyError(f"unknown artifact_type: {artifact_type}")
    validate_episode_id(episode_id)
    if generation <= 0 or version <= 0:
        raise StorageKeyError("generation and version must be positive")
    _segment, stem, suffix, _content_type = spec
    return f"{_FILE_PREFIX}{_SEGMENT}/{episode_id}/{spec[0]}/g{generation}_{stem}_v{version}{suffix}"


def parse_storage_key(key: str) -> StorageKeyRef:
    """Parse a storage_key back to its identity; reject escapes (pure, no IO)."""
    if not key.startswith(_FILE_PREFIX):
        raise StorageKeyError("storage_key must carry the file: prefix")
    relative = key[len(_FILE_PREFIX) :]
    if not relative or relative.startswith("/") or "\\" in relative:
        raise StorageKeyError("storage_key relative path must be slash-separated and relative")
    parts = relative.split("/")
    if len(parts) != 4 or parts[0] != _SEGMENT:
        raise StorageKeyError(
            f"storage_key must follow episodes/{{episode_id}}/{{segment}}/{{filename}}: {key}"
        )
    _episodes, episode_id, segment, filename = parts
    if any(part in {"", ".", ".."} for part in parts):
        raise StorageKeyError("storage_key path segments must not be empty or traversing")
    validate_episode_id(episode_id)
    match = _FILENAME_PATTERN.match(filename)
    if match is None:
        raise StorageKeyError(f"storage_key filename does not follow the naming rule: {filename}")
    artifact_type = _STEM_TO_TYPE.get(match.group("stem"))
    if artifact_type is None or _STEM_BY_TYPE[artifact_type][0] != segment:
        raise StorageKeyError(f"storage_key segment {segment} does not match the filename")
    return StorageKeyRef(
        episode_id=episode_id,
        artifact_type=artifact_type,
        generation=int(match.group("generation")),
        version=int(match.group("version")),
    )