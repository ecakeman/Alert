# P6 收尾交付 · 证据索引

> 口径：本批为 **11 个具名 task**（MS-P5.9/P5.10/P5.11 + MS-P6.1~P6.8 = 3+8）与 **3 类子判据**；
> 8 个 P6 task 严格串行、一 task 一个本地 commit（提交信息含 MS-P6.x），整批完成后一次性覆盖式 push。

## 已完成（本地 commit）
| task | 提交 | 状态 |
|---|---|---|
| MS-P6.1（含 D-24 `P6.1-PRE`） | 8ae2606 | 完成（7 路由整文件 diff 空 + PRE-1~8） |
| MS-P6.2 | ee9c4cc | 完成（dashboard 24 桶/分布/批量端点） |
| MS-P6.3 | c316230 | 完成（2/4/11/12 聚合、无 N+1、形状对齐） |
| MS-P6.4 | 183a7e9 | 完成（14/15 形状；非空时序稳定排序为 GAP） |
| MS-P6.5 | e44158d | 完成（bridge 4/4；D-28 排除 system-core） |
| MS-P6.6 | 17b1892 | 完成（删 207 文件、7 module 构建、structure-only 9/边界 PASS） |
| MS-P6.7 | — | **未完成（GAP）**：冷启动全拓扑未执行 |
| MS-P6.8 | — | **部分完成**：demo minimal 可跑；full 422、文档最终态未完成 |

## 已知偏差登记
- D-24 契约保真收口（registry L1 漂移，P6.1-PRE）
- D-25 seed-baseline.sh 域 schema 适配（白名单外必需）
- D-26 task 批量端点内部路径 `/internal/batch/*`（gin 通配冲突）
- D-27 main.go 注入 Clients.Task（白名单外必需）
- D-28 bridge 配置零命中排除 system-core（P6.6 删除后归零）
- D-29 system-core 残余引用仅注释/pb 元数据/历史脚本（豁免）

## 机械口径
- `bash scripts/verify-p6.sh --only=direct`：7 条直通逐字节 diff + D-24 结构断言 + 入口唯一。
- `bash scripts/verify-p6.sh --only=bridge`：旧通道 4 段断言。
- D-18：P6.1 只承诺 7 条直通整文件一致；02/04/11/12 归 P6.3、01 归 P6.2、14/15 归 P6.4。
- D-23：数值层按“形状层 + 固定 ID 条目层 + 当前库数值层”判定。