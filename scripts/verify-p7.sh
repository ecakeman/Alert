#!/usr/bin/env bash
# P7 结构搬迁门禁（MS-P4.4H / design 2.9.7）。
# 七段：1 结构 / 2 构建 / 3 路径 / 4 边界 / 5 编排 / 6 证据 / 7 Python。
# 参数：--structure-only 只跑结构类断言；--no-docker 跳过编排运行时；--json 输出机器可读结果。
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

GO_BIN=${GO_BIN:-}
if [ -z "$GO_BIN" ]; then
  for candidate in "$HOME/go-dist/go/bin/go" "$(command -v go 2>/dev/null || true)"; do
    if [ -n "$candidate" ] && [ -x "$candidate" ]; then GO_BIN="$candidate"; break; fi
  done
fi

# 断串构造，避免脚本被自身扫描命中（design 2.9.7 自排除要求）
OLD="serv""ices"
BASELINE_TAG=${BASELINE_TAG:-ms-p7-baseline}

STRUCTURE_ONLY=false
NO_DOCKER=false
JSON=false
for arg in "$@"; do
  case "$arg" in
    --structure-only) STRUCTURE_ONLY=true ;;
    --no-docker) NO_DOCKER=true ;;
    --json) JSON=true ;;
    -h|--help)
      sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

PASS=0; FAIL=0; SKIP=0
declare -a RESULTS=()
ok()  { PASS=$((PASS+1)); RESULTS+=("PASS|$1|"); echo "PASS $1"; }
ng()  { FAIL=$((FAIL+1)); RESULTS+=("FAIL|$1|$2"); echo "FAIL $1 :: $2"; }
skp() { SKIP=$((SKIP+1)); RESULTS+=("SKIP|$1|$2"); echo "SKIP $1 :: $2"; }

hdr() { echo; echo "===== $1 ====="; }

# ---------- 段 1：结构 ----------
hdr "1 结构"
a1=$(git ls-files | grep -cE "^$OLD/" || true)
[ "$a1" = "0" ] && ok A1 || ng A1 "git 索引仍有 $a1 条旧路径"

if [ ! -d "$OLD" ]; then ok A2; else ng A2 "磁盘仍存在 $OLD/"; fi

if [ -n "$GO_BIN" ]; then
  a3=$("$GO_BIN" work edit -json | python3 -c 'import sys,json;d=json.load(sys.stdin);u=d.get("Use") or [];print(len(u), all(x.get("DiskPath","").startswith("./go/") for x in u))' 2>/dev/null || echo "0 False")
  set -- $a3
  if [ "${1:-0}" -ge 7 ] && [ "${2:-False}" = "True" ]; then ok A3; else ng A3 "go.work Use=$a3"; fi
else
  skp A3 "go 不可用"
fi

mods=$(ls -d go/services/*/go.mod 2>/dev/null | wc -l)
if [ -f go/contracts/go.mod ] && [ -f go/platform/go.mod ] && [ "$mods" -ge 5 ]; then
  ok "module-count(>=5)"
else
  ng "module-count(>=5)" "contracts/platform 缺失或 go/services module=$mods"
fi

link_count=$(find . -type l -not -path './.git/*' \
  -not -path './apps/console/node_modules/*' -not -path './python/agent-runtime/.venv/*' | wc -l)
[ "$link_count" = "0" ] && ok A17 || ng A17 "非豁免符号链接 $link_count 条"

# ---------- 段 3：路径 ----------
hdr "3 路径"
a5=$(grep -rn '=> \.\./\.\./go/' go/ 2>/dev/null | wc -l)
[ "$a5" = "0" ] && ok A5 || ng A5 "过渡 replace 残留 $a5 处"

if [ -n "$GO_BIN" ]; then
  # A6 的本质是“module 身份未被改名”：窗口后允许 import 增减（P6.6 删除 system-core 会下降），只断言前缀未改名且仍在使用。
  a6=$(grep -rho 'ecakeman/alert/services/' --include='*.go' go/ 2>/dev/null | wc -l)
  a6bad=$(grep -rn 'ecakeman/alert/go/' --include='*.go' go/ 2>/dev/null | wc -l)
  if [ "$a6bad" = "0" ] && [ "$a6" -gt 0 ]; then ok A6; else ng A6 "import=$a6（>0）改名前缀=$a6bad（应为 0）"; fi
else
  skp A6 "go 不可用"
fi

a7=$(grep -rnE "($ROOT/|\./|\.\./)?$OLD/[a-z_-]+/" scripts deploy --include='*.sh' 2>/dev/null \
  | grep -v 'go/services/' | grep -v 'api/v1/services' | grep -v 'verify-p7.sh' | wc -l)
[ "$a7" = "0" ] && ok A7 || ng A7 "脚本旧路径 $a7 处"

if [ "$STRUCTURE_ONLY" = true ]; then
  skp A8 "文档迁移在 S8（--structure-only 跳过）"
else
  a8=$(grep -rnE "$OLD/[a-z_-]+/" README.md docs/architecture docs/runbooks contracts --include='*.md' 2>/dev/null \
    | grep -v 'go/services' | grep -v 'api/v1/services' | grep -v 'path-migration.md' | wc -l)
  [ "$a8" = "0" ] && ok A8 || ng A8 "文档旧路径 $a8 处"
fi

a13=$(grep -rn "$OLD/agent-runtime" python README.md docs contracts \
  --include='*.md' --include='*.toml' --include='*.py' 2>/dev/null \
  | grep -v 'docs/evidence' | grep -v 'path-migration.md' | wc -l)
[ "$a13" = "0" ] && ok A13 || ng A13 "Python 旧路径 $a13 处"

if [ "$STRUCTURE_ONLY" = true ]; then
  echo
  echo "---- structure-only 结束：PASS=$PASS FAIL=$FAIL SKIP=$SKIP ----"
  [ "$FAIL" = "0" ] && exit 0 || exit 1
fi

# ---------- 段 2：构建 ----------
hdr "2 构建"
if [ -n "$GO_BIN" ]; then
  for m in go/contracts go/platform go/services/console-bff go/services/service-registry \
           go/services/monitor-svc go/services/episode-svc; do
    if (cd "$m" && "$GO_BIN" build ./... >/dev/null 2>&1); then ok "A-build $m"; else ng "A-build $m" "workspace build failed"; fi
    if (cd "$m" && GOWORK=off "$GO_BIN" build ./... >/dev/null 2>&1); then ok "B-build $m"; else ng "B-build $m" "GOWORK=off build failed"; fi
  done
else
  skp "build" "go 不可用"
fi

# ---------- 段 4：边界 ----------
hdr "4 边界"
if [ -n "$GO_BIN" ]; then
  if GO_BIN="$GO_BIN" bash scripts/verify-module-boundary.sh >/tmp/p7-boundary.log 2>&1; then
    ok A14
  else
    ng A14 "$(tail -1 /tmp/p7-boundary.log)"
  fi
else
  skp A14 "go 不可用"
fi

# ---------- 段 5：编排 ----------
hdr "5 编排"
a9=$(grep -c 'context: \.\./\.\./go' deploy/compose/compose.yaml deploy/compose/compose.p1.yaml \
  deploy/compose/compose.p2.yaml deploy/compose/compose.p3.yaml 2>/dev/null | awk -F: '$2==1{c++}END{print c+0}')
[ "$a9" = "4" ] && ok A9 || ng A9 "context 命中文件数=$a9（期望 4）"

a10=$(grep -rE '^context: \.\./\.\.$' deploy/compose/ 2>/dev/null | wc -l)
[ "$a10" = "0" ] && ok A10 || ng A10 "旧 context $a10 处"

a11=$(grep -c '^COPY platform platform' go/services/*/Dockerfile 2>/dev/null | awk -F: '$2==1{c++}END{print c+0}')
[ "$a11" = "4" ] && ok A11 || ng A11 "COPY platform 命中=$a11（期望 4）"

a12=$(grep -c '^WORKDIR /src/services/' go/services/*/Dockerfile 2>/dev/null | awk -F: '$2==1{c++}END{print c+0}')
[ "$a12" = "4" ] && ok A12 || ng A12 "WORKDIR 命中=$a12（期望 4）"

if [ "$NO_DOCKER" = true ] || ! command -v docker >/dev/null 2>&1; then
  skp A15 "docker 不可用或 --no-docker"
else
  if docker compose -f deploy/compose/compose.yaml config --format json >/tmp/p7-compose.json 2>/dev/null; then
    ok A15
  else
    ng A15 "compose config 失败"
  fi
fi

# ---------- 段 6：证据 ----------
hdr "6 证据"
if git rev-parse -q --verify "$BASELINE_TAG" >/dev/null 2>&1; then
  a4=$(git diff --stat "$BASELINE_TAG" -- docs/evidence/p0 docs/evidence/p1 docs/evidence/p2 docs/evidence/p3 | wc -l)
  [ "$a4" = "0" ] && ok A4 || ng A4 "冻结证据被改动 $a4 行"
else
  skp A4 "基线 tag $BASELINE_TAG 不存在"
fi

[ -f docs/architecture/path-migration.md ] && ok "path-migration" || ng "path-migration" "对照表缺失"

# ---------- 段 7：Python ----------
hdr "7 Python"
if [ -x python/agent-runtime/.venv/bin/ruff ] && [ -x python/agent-runtime/.venv/bin/pytest ]; then
  if (cd python/agent-runtime && .venv/bin/ruff check src eval tests >/tmp/p7-ruff.log 2>&1); then ok "A16-ruff"; else ng "A16-ruff" "$(tail -1 /tmp/p7-ruff.log)"; fi
  if (cd python/agent-runtime && .venv/bin/pytest tests -q >/tmp/p7-pytest.log 2>&1); then ok "A16-pytest"; else ng "A16-pytest" "$(tail -1 /tmp/p7-pytest.log)"; fi
else
  skp A16 "未执行（未重建 venv）"
fi

echo
echo "---- P7 门禁结束：PASS=$PASS FAIL=$FAIL SKIP=$SKIP ----"
if [ "$JSON" = true ]; then
  printf '{"results":['
  first=1
  for r in "${RESULTS[@]}"; do
    IFS='|' read -r st name detail <<< "$r"
    if [ "$first" = 0 ]; then printf ','; fi
    first=0
    printf '{"status":"%s","assertion":"%s","detail":"%s"}' "$st" "$name" "$detail"
  done
  printf ']}\n'
fi

[ "$FAIL" = "0" ] && exit 0 || exit 1