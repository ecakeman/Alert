# P6 收尾交付 · 证据索引

> 口径：本批为 **11 个具名 task**（MS-P5.9/P5.10/P5.11 + MS-P6.1~P6.8 = 3+8）与 **3 类子判据**；
> 8 个 P6 task 严格串行、一 task 一个本地 commit（提交信息含 `MS-P6.x`），整批完成后一次性覆盖式 push。

## 偏差登记

- **D-24 契约保真收口**：P5 拆分后 registry 的 L1 输出漂移（07 `targets` 裸数组、target 多 `config_generation`、service 视图缺 `target_count`/`last_check_at`/`active_episode_id`）。作为 **MS-P6.1 前置子步骤 `P6.1-PRE`** 并入 P6，与 P6.1 同一 commit；不新增具名 task / commit / 子判据类别（spec 8.2.4.5）。

## task 证据文件

| task | 证据文件 | 状态 |
|---|---|---|
| MS-P6.1（含 `P6.1-PRE`） | `p6.1.md`、`routes61/`、`routes61.drift/`、`p6.1-known-diff.txt` | 见 p6.1.md |
| MS-P6.2 | `p6.2.md` | 待施工 |
| MS-P6.3 | `p6.3.md` | 待施工 |
| MS-P6.4 | `p6.4.md` | 待施工 |
| MS-P6.5 | `p6.5.md` | 待施工 |
| MS-P6.6 | `p6.6.md` | 待施工 |
| MS-P6.7 | `p6.7.md` | 待施工 |
| MS-P6.8 | `p6.8.md` | 待施工 |

## 机械口径

- `bash scripts/verify-p6.sh --only=direct`：7 条直通逐字节 diff + D-24 三条结构断言 + 入口唯一。
- D-18：P6.1 只承诺 7 条直通整文件一致；02/04/11/12 归 P6.3、01 归 P6.2、14/15 归 P6.4、P6.7 复验 18 路由。
- D-23：数值层按“形状层 + 固定 ID 条目层 + 当前库数值层”三层判定，不硬编码历史总数。