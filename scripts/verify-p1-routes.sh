#!/usr/bin/env bash
# P1.5：经 BFF 采集 18+3 响应，与 P0 直连单体基线逐文件比对。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
OUT="${OUT:-$ROOT/docs/evidence/p1/routes}"
BASELINE="${BASELINE:-$ROOT/docs/evidence/p0/routes}"

if [ ! -d "$BASELINE" ]; then
  echo "verify-p1-routes: P0 基线不存在：$BASELINE"
  exit 2
fi

BASE_URL="$BASE_URL" OUT_DIR="$OUT" bash "$ROOT/scripts/snapshot-routes.sh"
bash "$ROOT/scripts/snapshot-diff.sh" "$OUT" "$BASELINE"