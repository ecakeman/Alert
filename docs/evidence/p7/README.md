# P7 结构搬迁验收证据（MS-P4.4J / S9）

## 1. 结论（三态）

| 状态 | 数量 |
|---|---|
| PASS | 34 |
| FAIL | 0 |
| SKIP | 0 |

`bash scripts/verify-p7.sh` 退出 0；结构搬迁窗口（MS-P4.4A~J）验收通过。

## 2. 锚点

| tag / 提交 | 值 |
|---|---|
| `ms-p7-p44` | `cd4e0f2`（P4.4 完成态） |
| `ms-p7-baseline` | `65e110c`（S0 之后） |
| S8 头 | `0b83f5f`（演练前进锚点） |
| `ms-p7-done` | S9 提交（本文件所在提交） |

## 3. 搬迁窗口提交（8 个）

```
0b83f5f docs(p7): 更新文档路径并新增路径变更对照表（MS-P4.4I）
ace906a chore(p7): 迁移门禁与搬迁脚本的代码路径并新增 verify-p7（MS-P4.4H）
558e54d refactor(p7): Docker 构建上下文收敛到 go/ 并调整 Dockerfile COPY（MS-P4.4G）
9516ff0 refactor(p7): 将 agent-runtime 迁入 python/ 并重建 venv（MS-P4.4F）
647a742 refactor(p7): 将 system-core 迁入 go/services（MS-P4.4E）
6353aa8 refactor(p7): 将四个域服务迁入 go/services 并归零过渡 replace（MS-P4.4D）
d0e7673 refactor(p7): 将 platform 迁入 go/ 并改用过渡 replace（MS-P4.4C）
b338b12 refactor(p7): 将 contracts 迁入 go/ 并保留 module 路径（MS-P4.4B）
```

## 4. 双向回退演练

1. 演练前全量门禁：`PASS=34 FAIL=0 SKIP=0`。
2. `git reset --hard ms-p7-baseline` → HEAD `65e110c`；旧路径 `GOWORK=off` 构建 **7/7 PASS**；`docker compose config` PASS；`git status` 洁净。
3. `git reset --hard 0b83f5f` 前进 → 重跑全量门禁 `PASS=34 FAIL=0 SKIP=0`，与第 1 步逐行一致（`diff` 退出 0）。
4. 说明：首次全量门禁发现 A13 误报新增《路径变更对照表》中的旧路径；已在 S9 为 A13 增加与 A8 一致的豁免，并复跑全绿。

结论：双向演练完成，结构可整体回退且前向恢复后门禁结果一致。

## 5. 已知遗留（不阻塞解冻）

| 项 | 处置 |
|---|---|
| R31：`episode-svc/Dockerfile` 缺 `COPY contracts` | 移交 MS-P4.9，并纳入 P4.10 门禁 |
| monitor / episode 端口口径与设计表不一致 | 移交 MS-P4.9 裁定；P7 仅保证与搬迁前一致 |

## 6. `verify-p7.sh` 全量输出

```
===== 1 结构 =====
PASS A1
PASS A2
PASS A3
PASS module-count(5)
PASS A17

===== 3 路径 =====
PASS A5
PASS A6
PASS A7
PASS A8
PASS A13

===== 2 构建 =====
PASS A-build go/contracts
PASS B-build go/contracts
PASS A-build go/platform
PASS B-build go/platform
PASS A-build go/services/console-bff
PASS B-build go/services/console-bff
PASS A-build go/services/service-registry
PASS B-build go/services/service-registry
PASS A-build go/services/monitor-svc
PASS B-build go/services/monitor-svc
PASS A-build go/services/episode-svc
PASS B-build go/services/episode-svc
PASS A-build go/services/system-core
PASS B-build go/services/system-core

===== 4 边界 =====
PASS A14

===== 5 编排 =====
PASS A9
PASS A10
PASS A11
PASS A12
PASS A15

===== 6 证据 =====
PASS A4
PASS path-migration

===== 7 Python =====
PASS A16-ruff
PASS A16-pytest

---- P7 门禁结束：PASS=34 FAIL=0 SKIP=0 ----
```
