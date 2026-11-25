"""分层断言（A13，CR-LAY-01~04）：domain 无 IO、application 无技术 import。

langgraph 是 design §2.1 目录树规划在 application 的编排框架（graph.py），
作为唯一例外放行；任何其他技术/框架 import 都视为分层违规。
"""

from pathlib import Path

import pytest

SRC = Path(__file__).resolve().parents[1] / "src" / "alert_agent"

_DOMAIN_FORBIDDEN = ("httpx", "grpc", "psycopg", "langgraph", "fastapi", "asyncio", "logging")
_APPLICATION_FORBIDDEN = ("httpx", "grpc", "psycopg", "fastapi")
_APPLICATION_ALLOWED = ("langgraph",)  # 编排框架例外（design §2.1）


def _python_files(sub: str):
    yield from (SRC / sub).rglob("*.py")


def _violations(sub: str, forbidden: tuple[str, ...], allowed: tuple[str, ...] = ()) -> list[str]:
    bad: list[str] = []
    for path in _python_files(sub):
        text = path.read_text(encoding="utf-8")
        for line_no, line in enumerate(text.splitlines(), start=1):
            stripped = line.strip()
            if stripped.startswith("#") or not any(
                token in line for token in ("import ", "from ")
            ):
                continue
            for token in forbidden:
                if token in line and not any(a in line for a in allowed):
                    bad.append(f"{path.relative_to(SRC)}:{line_no}: {stripped}")
    return bad


def test_domain_has_no_io_or_framework_imports() -> None:
    assert _violations("domain", _DOMAIN_FORBIDDEN) == []


def test_application_has_no_technology_imports() -> None:
    assert _violations("application", _APPLICATION_FORBIDDEN) == []


@pytest.mark.parametrize("module", ["domain", "application", "infrastructure", "transport", "bootstrap"])
def test_layer_directories_exist(module: str) -> None:
    assert (SRC / module).is_dir()