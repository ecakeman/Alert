"""storage_key 地址制测试（CR-GW-09，design 4.6）：往返一致与穿越拒绝。"""

import pytest

from alert_agent.domain.shared.storage_key import (
    StorageKeyError,
    build_storage_key,
    parse_storage_key,
)


def test_round_trip_all_artifact_types() -> None:
    for artifact_type, suffix in (("INVESTIGATION", ".md"), ("DIAGNOSIS_REPORT", ".json")):
        key = build_storage_key("ep1", artifact_type, 2, 3)
        assert key.startswith("file:episodes/ep1/diagnosis/g2_")
        assert key.endswith(suffix)
        ref = parse_storage_key(key)
        assert (ref.episode_id, ref.artifact_type, ref.generation, ref.version) == (
            "ep1",
            artifact_type,
            2,
            3,
        )


@pytest.mark.parametrize(
    "episode_id",
    ["../evil", "/abs/path", "a\\b", "", "e/../../x"],
)
def test_build_rejects_traversal_episode_ids(episode_id: str) -> None:
    with pytest.raises(StorageKeyError):
        build_storage_key(episode_id, "INVESTIGATION", 1, 1)


@pytest.mark.parametrize(
    "key",
    ["not-a-key", "file:/abs/path.md", "file:episodes/e1/secret/g2_investigation_v1.md", "episodes/e1/diagnosis/g1_investigation_v1.md"],
)
def test_parse_rejects_malformed_keys(key: str) -> None:
    with pytest.raises(StorageKeyError):
        parse_storage_key(key)