"""Minimal offline eval (T24, D8, R9, CR-DISC-07).

Standalone tool: reads a small CSV dataset, runs the retrieval chain (same
T16/T17 wiring as production -- no second implementation), and judges each
retrieval against the gold answer with the structured LlmJson path (CR-REG-13).
The dataset columns must match exactly what this script consumes; a missing
column aborts at startup (R9). No coverage targets, no claims about
unexecuted evaluations (CR-DISC-07): the script reports only what it ran.

Usage:
    python eval/run.py --dataset eval/datasets/simple_qa.csv

Requires the real Postgres/LLM environment (not part of the smoke-free build
phase; see T25).
"""

from __future__ import annotations

import argparse
import asyncio
import csv
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from pydantic import BaseModel, Field

from alert_agent.bootstrap.wiring import build_llm_client, build_retrieval_searcher
from alert_agent.config import load_settings

REQUIRED_COLUMNS = ("question", "gold_answer")

_JUDGE_SYSTEM = (
    "你是检索质量评审。给定问题、标准答案与检索到的资料，"
    "评估资料是否足以支撑标准答案。输出 JSON：score（0~10 整数）与 reason（简体中文，一句话）。"
)


class JudgeScore(BaseModel):
    score: int = Field(ge=0, le=10)
    reason: str = Field(min_length=1)


def load_dataset(path: Path) -> list[dict[str, str]]:
    """Read the CSV and abort on missing required columns (R9)."""
    if not path.is_file():
        print(f"dataset not found: {path}", file=sys.stderr)
        raise SystemExit(2)
    with path.open(encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        columns = reader.fieldnames or []
        missing = [column for column in REQUIRED_COLUMNS if column not in columns]
        if missing:
            print(
                f"dataset is missing required columns: {missing} (R9); found: {columns}",
                file=sys.stderr,
            )
            raise SystemExit(2)
        return [
            {key: (row.get(key) or "").strip() for key in REQUIRED_COLUMNS}
            for row in reader
            if any((row.get(key) or "").strip() for key in REQUIRED_COLUMNS)
        ]


async def evaluate(dataset_path: Path) -> int:
    settings = load_settings()
    from psycopg_pool import AsyncConnectionPool

    pool = AsyncConnectionPool(
        conninfo=settings.shared.database_url_cognitive,
        open=False,
    )
    await pool.open()
    try:
        searcher = build_retrieval_searcher(settings, pool)
        judge = build_llm_client(settings)
        rows = load_dataset(dataset_path)
        results: list[dict] = []
        for row in rows:
            retrieval = await searcher.search(query=row["question"])
            context = "\n".join(
                f"[{index + 1}] {chunk.content[:400]}"
                for index, chunk in enumerate(retrieval.chunks)
            ) or "（无检索结果）"
            try:
                verdict = await judge.complete_json(
                    JudgeScore,
                    system=_JUDGE_SYSTEM,
                    user=(
                        f"[问题]\n{row['question']}\n\n[标准答案]\n{row['gold_answer']}\n\n"
                        f"[检索资料]\n{context}"
                    ),
                )
                score, reason = verdict.score, verdict.reason
            except Exception as exc:  # noqa: BLE001 -- one row failing must not stop the run
                score, reason = 0, f"judge failed: {type(exc).__name__}"
            results.append(
                {
                    "question": row["question"],
                    "gold_answer": row["gold_answer"],
                    "score": score,
                    "reason": reason,
                    "hits": len(retrieval.chunks),
                    "leg_status": retrieval.leg_status,
                }
            )
            print(f"[{len(results)}/{len(rows)}] {row['question']} -> {score}/10")
        average = (
            sum(item["score"] for item in results) / len(results) if results else 0.0
        )
        summary = {"rows": len(results), "average_score": round(average, 2), "results": results}
        print(json.dumps(summary, ensure_ascii=False, indent=2))
        return 0
    finally:
        await pool.close()


def main() -> int:
    parser = argparse.ArgumentParser(description="minimal offline retrieval eval (T24)")
    parser.add_argument(
        "--dataset",
        default=str(ROOT / "eval" / "datasets" / "simple_qa.csv"),
        help="CSV with question/gold_answer columns (R9)",
    )
    args = parser.parse_args()
    return asyncio.run(evaluate(Path(args.dataset)))


if __name__ == "__main__":
    raise SystemExit(main())