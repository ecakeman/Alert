"""pytest 路径配置：把 src 布局加入 sys.path，使测试可直接导入 alert_agent。"""

import sys
from pathlib import Path

SRC = Path(__file__).resolve().parents[1] / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))