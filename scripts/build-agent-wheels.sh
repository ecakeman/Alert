#!/usr/bin/env bash
# 生成 agent-runtime 离线 wheel 输入（不入库；见 .gitignore）。
set -euo pipefail
cd "$(dirname "$0")/../python/agent-runtime"
mkdir -p wheels
python3 -c "import tomllib;d=tomllib.load(open('pyproject.toml','rb'));print('\n'.join(d['project']['dependencies']))" > /tmp/agent-requirements.txt
PIP="$HOME/.local/bin/pip"
if [ ! -x "$PIP" ]; then PIP="python3 -m pip"; fi
$PIP download --dest wheels -r /tmp/agent-requirements.txt -i https://pypi.org/simple
$PIP download --dest wheels setuptools wheel -i https://pypi.org/simple
count=$(find wheels -maxdepth 1 -type f | wc -l)
bytes=$(du -sb wheels | awk '{print $1}')
echo "wheels=$count bytes=$bytes"