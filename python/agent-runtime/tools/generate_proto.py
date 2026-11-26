"""Regenerate Python gRPC stubs from the single contract source.

Contract source (read-only, never copied): ``contracts/proto/alertflow/...``
Output (committed, never hand-edited):
    ``python/agent-runtime/src/alert_agent/infrastructure/grpc/gen/``

Usage:

    python/agent-runtime/.venv/bin/python python/agent-runtime/tools/generate_proto.py

Generated stubs were produced with grpcio-tools ``GRPC_TOOLS_PIN``; the
runtime dependency family in ``pyproject.toml`` (grpcio / grpcio-tools /
protobuf) is locked to the same family so imports never hit a
GRPC_GENERATED_VERSION mismatch (CR-REG-48). The script rewrites the
proto-package imports inside generated files to the in-repo
``alert_agent.infrastructure.grpc.gen`` prefix; it never edits the proto
sources (CR-GW-01) and never writes outside the gen directory (CR-DATA-06).
"""

from __future__ import annotations

import importlib.metadata
import re
import shutil
import subprocess
import sys
from pathlib import Path

GRPC_TOOLS_PIN = "1.62.3"
PROTOBUF_FAMILY = (4, 25)
GEN_PACKAGE_PREFIX = "alert_agent.infrastructure.grpc.gen"

PROTO_FILES = (
    "alertflow/agent/v1/agent_task.proto",
    "alertflow/system/v1/common.proto",
    "alertflow/system/v1/task_gateway.proto",
)

GEN_PACKAGE_DIRS = (
    "alertflow/agent/v1",
    "alertflow/system/v1",
)

_IMPORT_PATTERN = re.compile(r"^(import|from)\s+alertflow\.", re.MULTILINE)


def _find_repo_root(start: Path) -> Path:
    for candidate in (start, *start.parents):
        if (candidate / "contracts" / "proto").is_dir():
            return candidate
    raise SystemExit(
        "could not locate repository root containing contracts/proto "
        f"(started at {start})"
    )


def _check_toolchain() -> str:
    tools_version = importlib.metadata.version("grpcio-tools")
    if tools_version != GRPC_TOOLS_PIN:
        raise SystemExit(
            f"grpcio-tools {tools_version} is installed but this script is pinned to "
            f"{GRPC_TOOLS_PIN}; install the pinned version before regenerating "
            "(CR-REG-48: keep generated code and runtime deps in one version family)."
        )
    protobuf_version = importlib.metadata.version("protobuf")
    major_minor = tuple(int(part) for part in protobuf_version.split(".")[:2])
    if major_minor != PROTOBUF_FAMILY:
        raise SystemExit(
            f"protobuf {protobuf_version} is installed but the pinned family is "
            f"{PROTOBUF_FAMILY[0]}.{PROTOBUF_FAMILY[1]}.x; align the family before "
            "regenerating (CR-REG-48)."
        )
    return tools_version


def _ensure_package_inits(gen_dir: Path) -> None:
    for rel in ("", *GEN_PACKAGE_DIRS):
        target = gen_dir / rel
        target.mkdir(parents=True, exist_ok=True)
        init_file = target / "__init__.py"
        init_file.touch(exist_ok=True)


def _rewrite_imports(gen_dir: Path) -> int:
    """Rewrite absolute proto-package imports to the in-repo gen package."""

    def _replace(match: re.Match[str]) -> str:
        return f"{match.group(1)} {GEN_PACKAGE_PREFIX}.alertflow."

    rewritten = 0
    for pb2_file in sorted(gen_dir.rglob("*_pb2*.py")):
        original = pb2_file.read_text(encoding="utf-8")
        updated = _IMPORT_PATTERN.sub(_replace, original)
        if updated != original:
            pb2_file.write_text(updated, encoding="utf-8")
            rewritten += 1
    return rewritten


def main() -> int:
    repo_root = _find_repo_root(Path(__file__).resolve())
    proto_dir = repo_root / "contracts" / "proto"
    gen_dir = (
        repo_root
        / "services"
        / "agent-runtime"
        / "src"
        / "alert_agent"
        / "infrastructure"
        / "grpc"
        / "gen"
    )

    tools_version = _check_toolchain()

    if gen_dir.exists():
        shutil.rmtree(gen_dir)
    gen_dir.mkdir(parents=True)

    command = [
        sys.executable,
        "-m",
        "grpc_tools.protoc",
        f"--proto_path={proto_dir}",
        f"--python_out={gen_dir}",
        f"--grpc_python_out={gen_dir}",
        *PROTO_FILES,
    ]
    subprocess.run(command, check=True, cwd=repo_root)

    _ensure_package_inits(gen_dir)
    touched = _rewrite_imports(gen_dir)

    print(f"grpcio-tools {tools_version} generated {len(PROTO_FILES)} stubs into {gen_dir}")
    print(f"rewrote imports to {GEN_PACKAGE_PREFIX}.* in {touched} file(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())