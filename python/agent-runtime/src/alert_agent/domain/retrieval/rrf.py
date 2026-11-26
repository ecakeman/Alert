"""倒数排名融合（RRF），以纯函数实现（design 5.1，CR-RAG-04）。

每个排名列表为每个位置贡献 ``1 / (k + rank)``；融合后的
分数决定最终候选的顺序。多查询扩展可自然适配：
每个扩展查询形成自己的向量路排名列表并参与
同一融合，且排名位置按列表各自独立（不存在拼接位置的
语义，CR-REG-20）。纯函数，无 IO。
"""

from __future__ import annotations

_RRF_K = 60


def rrf_fuse(
    rankings: list[list[str]], *, k: int = _RRF_K, top_k: int = 10
) -> list[tuple[str, float]]:
    """将各路的 chunk-id 排名融合为一个 top-K。

    返回 ``[(chunk_id, fused_score)]``，按分数降序排列，
    截断至 ``top_k``。跨路重复的 id 会累加分数——
    这正是 RRF 的语义，而非缺陷。
    """
    scores: dict[str, float] = {}
    for ranking in rankings:
        for position, chunk_id in enumerate(ranking, start=1):
            scores[chunk_id] = scores.get(chunk_id, 0.0) + 1.0 / (k + position)
    ordered = sorted(scores.items(), key=lambda item: (-item[1], item[0]))
    return ordered[:top_k]