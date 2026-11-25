#!/usr/bin/env bash
# 阶段回退骨架：--to P0 实装 R1（停新服务）/R5（恢复旧进程）；
# P2~P5 的 R2~R4 尚未实现，显式返回非零，禁止"假回退"。
set -euo pipefail

SPLIT_DIR="$(cd "$(dirname "$0")" && pwd)"
STATE_FILE="$SPLIT_DIR/.split-state.json"
COMPOSE_FILE="$SPLIT_DIR/../compose/compose.yaml"

TARGET="P0"
DRY_RUN=false
CONFIRM=false
RESTORE=false
while [ $# -gt 0 ]; do
  case "$1" in
    --to) TARGET="${2:?--to 需要值}"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    --confirm) CONFIRM=true; shift ;;
    --restore) RESTORE=true; shift ;;
    *) echo "未知参数: $1"; exit 2 ;;
  esac
done

if [ -f "$STATE_FILE" ]; then
  state="$(cat "$STATE_FILE")"
else
  state='{"stage":"P0","phase":"not-started","started_at":"","snapshot":""}'
fi
echo "当前状态：$state"

run_or_echo() {
  local desc="$1"
  shift
  if [ "$DRY_RUN" = true ]; then
    echo "[dry-run] $desc: $*"
    return 0
  fi
  if [ "$CONFIRM" != true ]; then
    echo "拒绝执行危险动作（缺少 --confirm）：$desc"
    exit 4
  fi
  echo "[执行] $desc"
  "$@"
}

case "$TARGET" in
  P1)
    # P1 回退：停 BFF + 控制台指回单体；--restore 恢复 P1 链路。
    NGINX_FILE="$SPLIT_DIR/../../apps/console/nginx.conf"
    if [ "$RESTORE" = true ]; then
      run_or_echo "R1' 启动 BFF" docker compose -f "$COMPOSE_FILE" up -d console-bff
      run_or_echo "R2' 控制台指回 BFF" sed -i 's|http://system-core:8080|http://console-bff:8080|' "$NGINX_FILE"
    else
      run_or_echo "R1 停 BFF" docker compose -f "$COMPOSE_FILE" stop console-bff
      run_or_echo "R2 控制台指回单体" sed -i 's|http://console-bff:8080|http://system-core:8080|' "$NGINX_FILE"
    fi
    ;;
  P0)
    # R1：P0 尚无新服务，记录为 no-op。
    echo "R1 停新服务：P0 无新服务（no-op）"
    # R5：恢复旧进程（system-core api/worker），由操作者显式启动。
    if [ "$DRY_RUN" = true ]; then
      echo "[dry-run] R5 恢复旧进程：go run ./services/system-core/cmd/api 与 cmd/worker"
    else
      echo "R5 恢复旧进程：请按 README 启动 system-core api/worker（脚本不自动拉起裸进程）"
    fi
    ;;
  P2|P3|P4|P5)
    echo "未实现阶段：$TARGET 的 R2~R4 尚未实装（按 tasks.md 在该阶段任务内补齐）"
    exit 3
    ;;
  *)
    echo "未知目标阶段：$TARGET（支持 P0|P1|P2|P3|P4|P5）"
    exit 2
    ;;
esac

echo "rollback: $TARGET 完成（dry_run=$DRY_RUN restore=$RESTORE）"