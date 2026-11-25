"""Reciprocal Rank Fusion as a pure function (design 5.1, CR-RAG-04).

Every rank list contributes ``1 / (k + rank)`` for each position; fused
scores order the final candidates. Multi-query expansion works naturally:
each expanded query forms its own vector-leg ranking list that participates
in the same fusion, and rank positions stay per-list (no concatenated-position
semantics, CR-REG-20). Pure, no IO.
"""

from __future__ import annotations

_RRF_K = 60


def rrf_fuse(
    rankings: list[list[str]], *, k: int = _RRF_K, top_k: int = 10
) -> list[tuple[str, float]]:
    """Fuse per-leg chunk-id rankings into one fused top-K.

    Returns ``[(chunk_id, fused_score)]`` ordered by descending score,
    truncated to ``top_k``. Duplicate ids across legs accumulate scores --
    that is the RRF semantics, not a bug.
    """
    scores: dict[str, float] = {}
    for ranking in rankings:
        for position, chunk_id in enumerate(ranking, start=1):
            scores[chunk_id] = scores.get(chunk_id, 0.0) + 1.0 / (k + position)
    ordered = sorted(scores.items(), key=lambda item: (-item[1], item[0]))
    return ordered[:top_k]