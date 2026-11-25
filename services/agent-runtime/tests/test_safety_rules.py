"""摄取安全规则测试（CR-REG-36，5.3.4/5.3.5）：路径穿越与 SSRF 拒绝。"""

import pytest

from alert_agent.domain.ingestion.path_rules import (
    PathRuleError,
    resolve_within_root,
    validate_callback_url,
)


@pytest.mark.parametrize("relative", ["../secret.md", "/etc/passwd", "a\\b.md", "", "docs/../../x.md"])
def test_resolve_rejects_escaping_paths(relative: str, tmp_path) -> None:
    with pytest.raises(PathRuleError):
        resolve_within_root(tmp_path, relative)


def test_resolve_keeps_legitimate_relative_paths(tmp_path) -> None:
    target = resolve_within_root(tmp_path, "docs/manual.md")
    assert str(target).startswith(str(tmp_path.resolve()))


@pytest.mark.parametrize(
    "url",
    [
        "ftp://localhost/cb",           # scheme 不在 http/https
        "https://evil.example.com/cb",  # 非白名单域名
        "http://10.0.0.5/cb",           # 非白名单私网地址（SSRF）
        "https:///cb",                  # 缺 host
    ],
)
def test_callback_url_rejects_non_allowlisted_targets(url: str) -> None:
    with pytest.raises(PathRuleError):
        validate_callback_url(url, ("localhost", "127.0.0.1"))


def test_callback_url_accepts_allowlisted_targets() -> None:
    assert validate_callback_url("https://localhost:9000/cb", ("localhost",)) == "https://localhost:9000/cb"
    assert validate_callback_url("http://127.0.0.1/cb", ("localhost", "127.0.0.1"))