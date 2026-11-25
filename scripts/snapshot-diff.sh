#!/usr/bin/env bash
# 比较两次采集（或指定基线目录与 .prev）；任一差异非零退出。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CURRENT="${1:-$ROOT/docs/evidence/p0/routes}"
PREVIOUS="${2:-$CURRENT.prev}"

if [ ! -d "$CURRENT" ]; then
  echo "snapshot-diff: 目录不存在：$CURRENT"
  exit 2
fi
if [ ! -d "$PREVIOUS" ]; then
  echo "snapshot-diff: 无上一次采集：$PREVIOUS（先运行 snapshot-routes.sh 两次）"
  exit 3
fi

DIFF_OUT="$(mktemp)"
trap 'rm -f "$DIFF_OUT"' EXIT

if diff -ru "$PREVIOUS" "$CURRENT" > "$DIFF_OUT"; then
  echo "snapshot-diff: 无差异（$PREVIOUS vs $CURRENT）"
  exit 0
fi

echo "snapshot-diff: 发现差异（$PREVIOUS vs $CURRENT）"
head -120 "$DIFF_OUT"
exit 1