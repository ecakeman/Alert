"""storage_key 寻址规则，以纯函数实现（design 4.6，D5，CR-GW-09a）。

格式（已对照 Go ``internal/infrastructure/artifacts/reader.go`` 核实）：
``file:{relative-path}``，使用正斜杠。Go 读取器会剥离
``file:`` 前缀，清理相对路径并拒绝绝对路径、
``..`` 段以及任何逃逸出读取器根目录的内容；此处生成的 key
保持相对且以斜杠分隔，因此仍可被该读取器读取
（当其根目录指向共享存储目录时，design R4）。

Key 内嵌 episode、artifact 类型、generation 与版本——
``file:episodes/{episode_id}/diagnosis/g{generation}_{stem}_v{version}{suffix}``
——因此旧的 generation 永远无法覆盖新 generation 的文件
（CR-GW-09a，CR-REG-46）。本模块无 IO（CR-LAY-03）。
"""

from __future__ import annotations

import re

from pydantic import BaseModel, ConfigDict, Field

_FILE_PREFIX = "file:"
_SEGMENT = "episodes"
_STEM_BY_TYPE: dict[str, tuple[str, str, str, str]] = {
    # artifact_type -> (路径段, 文件名主干, 后缀, content_type)
    "INVESTIGATION": ("diagnosis", "investigation", ".md", "text/markdown"),
    "DIAGNOSIS_REPORT": ("diagnosis", "diagnosis_report", ".json", "application/json"),
}
_FILENAME_PATTERN = re.compile(
    r"^g(?P<generation>\d+)_(?P<stem>investigation|diagnosis_report)"
    r"_v(?P<version>\d+)\.(?P<suffix>md|json)$"
)
_STEM_TO_TYPE = {stem: artifact_type for artifact_type, spec in _STEM_BY_TYPE.items() for stem in (spec[1],)}


class StorageKeyError(ValueError):
    """storage_key 未通过寻址规则校验（稳定语义，6.1.3.4）。"""


class StorageKeyRef(BaseModel):
    """解析后的 storage_key 身份信息（design 4.6）。"""

    model_config = ConfigDict(frozen=True)

    episode_id: str = Field(min_length=1)
    artifact_type: str = Field(min_length=1)
    generation: int = Field(gt=0)
    version: int = Field(gt=0)


def content_type_of(artifact_type: str) -> str:
    """artifact 注册时登记的 content_type（D5）。"""
    spec = _STEM_BY_TYPE.get(artifact_type)
    if spec is None:
        raise StorageKeyError(f"unknown artifact_type: {artifact_type}")
    return spec[3]


def validate_episode_id(value: str) -> str:
    """在 episode id 进入 storage_key 之前，拒绝不适于路径的形式。

    Go 读取器的包含性检查仍是权威；这里是
    生成侧的第一道防线（5.3.4 纪律）。
    """
    if not value or value in {".", ".."} or value != value.strip():
        raise StorageKeyError(f"episode_id is not a usable single path segment: {value!r}")
    for forbidden in ("/", "\\", "\x00", "\n", "\r"):
        if forbidden in value:
            raise StorageKeyError(f"episode_id must not contain {forbidden!r}")
    return value


def build_storage_key(episode_id: str, artifact_type: str, generation: int, version: int) -> str:
    """为一个 artifact 版本构建 storage_key（纯函数，无 IO）。"""
    spec = _STEM_BY_TYPE.get(artifact_type)
    if spec is None:
        raise StorageKeyError(f"unknown artifact_type: {artifact_type}")
    validate_episode_id(episode_id)
    if generation <= 0 or version <= 0:
        raise StorageKeyError("generation and version must be positive")
    _segment, stem, suffix, _content_type = spec
    return f"{_FILE_PREFIX}{_SEGMENT}/{episode_id}/{spec[0]}/g{generation}_{stem}_v{version}{suffix}"


def parse_storage_key(key: str) -> StorageKeyRef:
    """将 storage_key 解析回其身份信息；拒绝逃逸形式（纯函数，无 IO）。"""
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