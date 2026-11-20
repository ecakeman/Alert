# AlertFlow 整改验收报告（方案第 12 节：12.1–12.7）

- **版本边界（固定历史节点，不把报告自身提交冒充当前 HEAD）**：
  - 被测代码基线：`460c6ab`（第三轮修复 PostgreSQL TCP readiness race 的代码提交，也是最后一个业务/部署代码变更提交）；
  - 报告首次入库：`7af5573`；
  - 后续报告修订：`83727c9`；
  - README 与验收报告对齐提交：`5311548`。
  - 说明：上述节点均为可追溯的历史提交；`460c6ab` 是被测代码基线，`7af5573`、`83727c9` 与 `5311548` 均只涉及文档。当前 `main` HEAD 应以 `git rev-parse origin/main` 或 GitHub 分支页面为准，不在验收报告中硬编码。
- **三轮口径**：
  - **第一轮（2026-11-15～17 上午）**：被测 `origin/main = d763fd1`，代码修复当时以 **30 项未提交改动**存在（23 modified + 7 untracked）—— §1.1、§1.3、§1.4、§2.1、§3.2、§5.1、§6.1 等「原始树」证据来自此轮；这些改动已于 17 日下午以 4 个提交入库（`c9a5d50`/`1c27947`/`39d8140`/`f2a30b1`）。
  - **第二轮（2026-11-17 晚）**：执行《AlertFlow整改执行指导-2026-11-17》，在 `f2a30b1` 之后完成 E2E 编排、缓存 query 键、Go 失效范围、镜像变量、Python wheel、K8s 文档六项，并**首次真实起栈连续跑通两次 E2E** —— §1.2、§3.1、§4.2、§5.3、§6、§8、§9、§11 已按此轮结果更新。本轮 6 个代码提交：`d55603c`、`3aba030`、`d2853fd`、`f4e5707`、`91f5a51`、`ab2b8dc`。
  - **第三轮（2026-11-17 晚收尾）**：执行《AlertFlow 收尾整改指导》，只做三件事——补 E2E 连续运行验收（`make e2e` 两次、均默认清理）、按实测更正本报告、整理远端分支。首次运行时暴露并修复一个 postgres 就绪竞态（`460c6ab`），修复后**两次 `make e2e` 均 exit 0（61 s / 56 s）且均自动清理**。本轮**没有架构/功能改动**。
- 基线对照：`1aa5b45`（monorepo 重排、整改提交之前）—— 用于判断 d763fd1 的失败是否由整改引入（§1.4）。
- 测试日期：2026-11-15 至 2026-11-17
- 测试环境：WSL2 Ubuntu 24.04 / Docker 29.6.1 + Compose v5.2.0 / Go 1.26.6 / JDK 25（Temurin 25.0.5+8）/ Python 3.12（uv）/ Node 22
- 口径：**只写实际执行过的结果**；未执行项单列于 §9，不计为通过；未复测项显式标注「未复测」。报告中的「测试通过」全部指**本地工作树/本地 Compose 栈**上的执行结果，**不是 CI 结果**（仓库没有 CI 流水线）。

> **对上一版报告的两处更正**
> 1. 删除「Go HTTP 面缺新增目标接口 `POST /api/v1/services/{id}/targets` → 405」——**误判**。接口实际存在：
>    `grep -n targets services/system-core/internal/api/server.go` → `:36` GET、`:37` POST，实测 201。
> 2. 「13 个整改提交」口径不准：`1aa5b45..d763fd1` 实为 **9 个**提交（`git log --oneline 1aa5b45..d763fd1 | wc -l` → 9），
>    `1aa5b45` 本身是 monorepo 目录重排。本报告改为「1aa5b45 基线 vs d763fd1 原始树 vs d763fd1+修复树」三栏。

---

## 0. 结论摘要

| 节 | 结果 | 来源 | 一句话 |
|---|---|---|---|
| 12.1 静态与单元测试 | 通过 | 第一、二轮完成修复；第三轮在 `460c6ab` 全量复跑 | `go test ./...` **0 FAIL**（7 个 ok 包、56 个 Test 函数）、Java **20/0/0**（含 `RevisionCacheTest` 6）、pytest 14 passed、Vue build exit 0；gofmt 剩 **2** 文件（均 `HEAD` 既存） |
| 12.2 契约一致性 | 部分修（有测试，无 HTTP 重放） | 两轮 | 19 条公开路由可用；#1–#4 已改且 405/404/409 有单测与真实测试库证据；**未做栈级 HTTP 端到端重放** |
| 12.3 E2E 主链路 | **通过（第三轮 `make e2e` 两次默认清理实测）** | 第二轮 + 第三轮 | 第二轮两次真实起栈 exit 0（65.19 s / 47.32 s），其中 runB 用 `ALERTFLOW_E2E_KEEP_STACK=1` 保留现场（**未清理**）；第三轮按收尾指导用 `make e2e` 连跑两次，**两次都默认清理**，均 exit 0（61 s / 56 s），全链路 + 双产物 + `ARCHIVED`，其他 project 不受影响 |
| 12.4 版本缓存 | 部分修（**已接口级验证**） | 第二轮 | query 进值键（SHA-256 前 16 hex）；不同 query 各自成键、revision 推进后旧值不命中、Redis 故障回源；5–6 s 短 TTL 与 Kafka 异步失效窗口仍在 |
| 12.5 运行与故障 | P0 **已修**（单测证据） | 两轮 | 三处根因已改 + 10 个监控回归用例全绿；**未做 pause 后自愈的栈级复现** |
| 12.6 镜像与部署 | 部分修（E2E 已重建镜像） | 第二轮 | `images.env.example` 已接入 Makefile/Compose/Dockerfile ARG，E2E 每次真实 `--build` 四个镜像；K8s 仍只做静态校验、未部署 |
| 12.7 验收 | **完成判定满足**（另有未执行项） | 三轮 | 指导文档第 10 节 8 条完成判定**全部满足**（单一入口、默认清理、两次 E2E 均 exit 0 且都验证到 `ARCHIVED` + 两个非空产物、query 不共用键、revision 后旧值不命中、Go 失效范围有计数测试、K8s 文档一致、报告版本边界记录固定历史节点：被测基线 `460c6ab`、报告首次入库 `7af5573`、后续修订 `83727c9`、README 与报告对齐 `5311548`；当前分支头以远端 ref 为准）；真实 K8s / 真实 LLM / `-race` / pause 自愈复现 / 契约 HTTP 重放等非判定项未执行（§9） |

阻塞项优先级一览（详见 §8）：

1. **P0**：监控调度永久停摆 —— **已修**（§5.3；确定性回归测试通过，**无栈级复现**）
2. **P1**：按 README 首条命令启动失败（空口令） —— **已修并实测**（第二轮以真实 `.env` 起栈两次成功；第三轮又用 `make e2e` 默认清理连跑两次成功，并修掉期间暴露的 postgres 就绪竞态 `460c6ab`；§3.1、§6.2）
3. **P1**：`RevisionCache` 读路径恒不命中 —— **已修并经接口级验证**（§4.2a；含本轮发现的「同 revision 下只缓存第一个 query」Lua 缺陷）
4. **P1**：写放大 —— **结论更正 + 已收窄**：`ApplyMonitorSample` 两个 `noteViews()` 调用点在互斥分支，受控测试实测普通采样只 bump `targets` **1** 条，**不是「每采样 10 条」**；元数据操作仍为 5 条，首次异常/恢复采样 5 条（§1.3、§4.2b）
5. **P2**：gofmt 2 文件（`HEAD` 既存）、无 ruff/OpenAPI 校验 —— **仍存**；Python 空 wheel **已修**（30,273 B，20 个模块）


> 已删除的条目：上一版 §0 第 3 条「Go 无新增目标接口（405）」、§8-B2「目标管理缺 Go 接口」—— 均系误判（接口存在，`server.go:37`，实测 201）。

---

## 1. 12.1 静态检查与单元测试

### 1.1 原始树（`d763fd1`，未叠加修复）

| 组件 | 命令 | 结果 |
|---|---|---|
| Go | `go build ./...` | exit 0；`cmd/system-core` 二进制 75 MB |
| Go | `go vet ./...` | exit 0 |
| Go | `gofmt -l .` | **FAIL：3 个文件** |
| Go | `go test -count=1 ./...` | **FAIL**：`api ok 2.23s`、`monitoring ok 0.169s`、`probe/http 0.283s`、`probe/kubernetes 0.015s`、`internal/system FAIL 4.824s` |
| Go | `go test -race` | **无法执行**：`-race requires cgo`，宿主无 gcc |
| Java | `mvn -B package` | BUILD SUCCESS；Tests run 11/0/0（Bridge 6 + VirtualThreadWeb 1 + Boundary 4）；jar 86.9 MB |
| Python | `uv run pytest -q` | **14 passed** |
| Python | `uv sync --frozen` / `uv lock --check` | exit 0 / Resolved 62 packages |
| Python | `uv build` | 成功但 **wheel 仅 1025 B**（只有 dist-info，无模块） |
| Python | `uv run python -c "import alertflow_agent"` | **ModuleNotFoundError**（必须 `PYTHONPATH=src`） |
| Vue | `vue-tsc --noEmit` | exit 0，0 诊断 |
| Vue | `npm run build` | exit 0；`DashboardView-*.js` 1043.08 kB、`index-*.js` 1219.00 kB |
| Compose | `make compose-config` | exit 0 |

### 1.2 修复后树（第一轮：`d763fd1` + 30 项未提交改动，第二轮已全部提交入库）

> 本节全部由 Lead 在**所有编辑停止后的单一代码状态**下重跑（第一轮 2026-11-17 13:40–13:50；第二轮在 `ab2b8dc` 上复跑，改动只涉及 deploy/integration/docs，Go 结果不变），日志见 §10。
> 第一轮的 30 项未提交改动已在 17 日下午以 `c9a5d50`/`1c27947`/`39d8140`/`f2a30b1` 入库；第二轮的 Java 用例数已更新为 20，gofmt 剩余文件数从 3 降为 2。
> **第三轮（收尾）在 `460c6ab` 上按同一套命令重跑**，结果与本节一致：`go vet`/`go build` exit 0、`go test -count=1 ./...` **7 ok / 0 FAIL**（system 7.506 s、api 2.408 s）、Java `Tests run: 20, Failures: 0, Errors: 0`、pytest **14 passed in 5.22 s**、`uv build` 仍 30,273 B、Vue **2266 modules**（`DashboardView-BaeayJ-X.js` 462.99 kB）、`make compose-config` exit 0、`bash -n deploy/e2e.sh` exit 0；gofmt 仍为同样的 2 个 `HEAD` 既存文件（日志 `/tmp/verify-final.log`）。

| 组件 | 命令 | 结果 |
|---|---|---|
| Go gofmt | `gofmt -l .` | 第一轮 3 个文件；第二轮 `internal/system/task.go` 已随修复格式化，**剩 2 个文件**：`cmd/system-core/artifacts.go`、`internal/gateway/server.go`。二者在 **HEAD 版本上同样未格式化**（`git show HEAD:<path> \| gofmt -l` 命中）→ **gofmt 差异是 `d763fd1` 既存问题，非本轮修复引入；本轮未修（P2 仍存）** |
| Go vet / build | `go vet ./...`；`go build ./...` | exit 0；exit 0 |
| Go 全量测试 | `go test -count=1 ./...` | **exit 0，0 FAIL**，7 个 ok：`internal/api 2.784s`、`internal/monitoring 0.193s`、`monitoring/probe/http 0.285s`、`monitoring/probe/kubernetes 0.020s`、`monitoring/scheduler 0.197s`、`monitoring/worker 0.020s`、`internal/system 5.778s`；其余 9 包 `[no test files]`。原始树的 `TestPublishPendingMarksSent` FAIL 已消失 |
| Go 重复稳定性 | `go test -count=3 ./internal/monitoring/...` | exit 0（scheduler/worker/engine 用例不 flaky） |
| Go 竞态检测 | `go test -race ...` | **无法执行**：`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`（宿主无 gcc）→ 见 §9-5 |
| Java 测试 | `JAVA_HOME=/home/sancho/tools/jdk-25.0.5+8 /home/sancho/tools/apache-maven-3.9.11/bin/mvn -B -s services/integration/maven-settings.xml -Dmaven.repo.local=/home/sancho/tools/m2repo -f services/integration/pom.xml test` | exit 0，**BUILD SUCCESS，Tests run: 20, Failures: 0, Errors: 0, Skipped: 0**（原始树 11 → 第一轮 14 → 第二轮 20）；分类：`BridgeAndInvocationTest 6`、`PublicApiErrorsTest 3`、`VirtualThreadWebTest 1`、`BoundaryAndVirtualThreadTest 4`、**`RevisionCacheTest 6`**（第二轮新增：命中/分 query/版本失效/空 query 稳定键/Redis 故障回源/query hash 形态） |
| Python 测试 | `uv sync --frozen && uv run pytest -q` | exit 0（Checked 59 packages）；**14 passed in 4.65s** |
| Vue 构建 | `npx vue-tsc --noEmit && npm run build` | exit 0；exit 0，**2266 modules，built in 9.41s**，产物 `DashboardView-BaeayJ-X.js 462.99 kB (gzip 155.81)`、`index-Com4i6yR.js 1,219.00 kB (gzip 392.30)`、`index-DCn_y49j.css 382.90 kB` |
| Compose 校验 | `ENV_FILE=deploy/compose/.env.example make compose-config` | exit 0，11 个 service 全解析（agent-runtime/db-init/fault/frontend/integration/kafka/kafka-init/nacos/postgres/redis/system-core） |

### 1.3 失败用例明细（原始树）

```
--- FAIL: TestPublishPendingMarksSent (0.24s)
    apply_test.go:403: unpublished 10 <nil>
```

根因（逐行核对）：`internal/system/apply.go` 的 `ApplyMonitorSample` 在提交前调用 `noteViews` 两次
（`apply.go:172`、`apply.go:194`），**但两处在互斥分支**——`:172` 是 `if ep == nil { return s.noteViews(...) }`，
`:194` 是处理完 Episode 后的 `return s.noteViews(...)`，任何一次采样只会走到其中一处。每次调用对
`dashboard`/`services`/`episodes`/`service:<id>`/`targets:<id>` 五个视图各写一条 `RESOURCE_REVISION` outbox，
因此**原始树普通采样是 5 条，不是「一次采样 10 条」**（旧版把两个互斥调用点重复计数了，本轮更正）。
而 `internal/system/publish.go` 的 `PublishPending` 只发 `AGENT_TASK_READY`，`UnpublishedCount` 统计 `published_at IS NULL` 全部行 → 断言失败。
运行态完整（`PublishByType("RESOURCE_REVISION")` 在 gateway 启动），**属测试未随 `562a01c` 同步，不是产品故障**。
本轮已按新语义重写该用例（§11-4）；修复后 `go test -count=1 ./internal/system/` → **ok 5.778s**，`TestPublishPendingMarksSent` 通过（§1.2）。
第二轮进一步把失效范围收窄到「实际可见变化的视图」，并新增受控计数用例（`revision_test.go`，§4.2b）：
普通采样 **1** 条（仅 `targets`）、首次异常建 Episode/恢复采样 **5** 条、APP 状态+版本变化 **4** 条、诊断完成状态不变 **0** 条、
解析 RESOLVED **3** 条、归档 ARCHIVED **2** 条。

gofmt 差异（原始树，均非功能问题）：

- `services/system-core/cmd/system-core/artifacts.go`、`services/system-core/internal/gateway/server.go`：import 分组顺序
- `services/system-core/internal/system/task.go:368`：`RegisterFaultDocumentInput` 字段对齐

修复后 gofmt 结果：第一轮剩 3 个文件，且三者在 **HEAD 版本上同样未格式化**（`git show HEAD:<path> | gofmt -l` 三份全部命中）
→ **gofmt 差异属 `d763fd1` 既存问题，非本轮修复引入；本轮未修（P2 仍存）**。
第二轮 `internal/system/task.go:368`（`RegisterFaultDocumentInput` 字段对齐）已随「归档路径补失效」的改动顺带格式化（`4a5f0a1` 前的 `d2853fd`），
`gofmt -l .` 在 `services/system-core` 下**只剩 2 个文件**（`artifacts.go`、`gateway/server.go`），二者仍未动、仍计入 P2。

### 1.4 基线对照 `1aa5b45`（整改提交之前）

| 组件 | `1aa5b45` 基线 | `d763fd1` 原始树 | 差异归因 |
|---|---|---|---|
| Go `gofmt -l` | 5 个文件有差异 | 3 个文件有差异 | 两处均非零，但**文件集合不同**；gofmt 不达标不是 d763fd1 新引入的孤立问题 |
| Go `go test ./...` | **全绿** | `internal/system` FAIL | **由整改提交引入**（`562a01c` 改 `PublishPending` 语义后测试未同步） |
| Java 测试 | 11 全绿 | 11 全绿 | 一致 |
| Python `pytest` | 14 全绿 | 14 全绿 | 一致 |

结论：d763fd1 唯一的测试退化为 Go 侧 `TestPublishPendingMarksSent`，**归因于整改提交而非代码缺陷本身**；修复后已转绿（`internal/system ok 5.778s`，§1.2）。

### 1.5 覆盖度（对照 §12.1 矩阵）

- 原始树：11 个 `_test.go`，`go test -list 'Test' ./...` 口径下 **37 个用例**；`scheduler`、`queue`、`worker`、`probe/tcp`、`gateway`、`transport`、`systemapi`、`db` **原始树无测试文件**（`internal/db/migrate_manual_test.go` 由 build tag 排除，未计入）。
- 修复后（第二轮结束）：**16 个 `_test.go` / 56 个 Test 函数**（**+19** = 第一轮 +14 + 第二轮 `revision_test.go` +5，`TestMain` 2 个不计）；`scheduler`、`worker` 从「无测试文件」变为 `ok`；`monitoring` 与 `internal/system` 显著加强。
- 矩阵点名场景中：队列满、同目标不重叠、配置更新生效、`Context` 取消 4 项有测试；**Worker 退出、调度竞争、结果提交失败原始树无测试**（修复后均已补，见下）。
- 实际漏网缺陷：**新建目标不采样**（§5.3），现已由 `TestEngineNewTargetGetsFirstSample` 覆盖。
- 第一轮补测（**14 个用例，全部通过**）：
  - `monitoring/scheduler`（新文件 5 个）：`TestSchedulerConcurrentSyncPopDeferKeepsHeapOrder`、`TestSchedulerSyncRaceKeepsInvariants`、`TestDeferRejectsStaleVersion`、`TestDeferAppliesCurrentVersion`、`TestRemoveThenPushResetsIndex` —— 覆盖「Sync/Defer/PopDue 并发时堆序与 `inHeap`/`index` 不变式」「过期版本 Defer 不得回堆」；包内断言 `requireHeapOrder`、`countInHeap`。`go test -count=1 ./internal/monitoring/scheduler/` → **ok 0.197s**，`-count=3` 亦 ok。
  - `monitoring/worker`（新文件 3 个）：`TestWorkerSurvivesPanic`、`TestWorkerNeverDiesAcrossManyPanics`、`TestRunWithoutOptionsStillRecovers` —— panic 不终止 worker，后续 job 仍被处理。**ok 0.020s**。
  - `monitoring`（引擎 2 个）：`TestEngineNewTargetGetsFirstSample`（新建目标在一个 interval 内出首个采样，覆盖漏网缺陷 §5.3）、`TestEngineSurvivesProbePanic`。**ok 0.193s**。
  - `internal/system`（新文件 4 个）：`TestMapDBErrUniqueViolationTo409`、`TestMapDBErrPassesThroughOthers`、`TestCreateServiceDuplicateKeyIsConflict`（0.31s）、`TestRetryUnknownEpisodeIsNotFound`（0.50s）。后两条打**真实测试库**（`127.0.0.1:55433`），分别验证重复 `service_key` → 409 `ALREADY_EXISTS`、未知 episode 的 `retry-diagnosis`/`retry-archive` → 404 `EPISODE_NOT_FOUND`。**ok 5.778s**（整个包）。
  - 第二轮 `internal/system/revision_test.go`（新文件 5 个）：按「单次增量」断言失效视图数 —— 普通采样 1、已有 Episode 普通采样 1、首次建 Episode/恢复 5、APP 状态+版本 4、诊断完成状态不变 0、解析 3、归档 2（§4.2b）。`go test -count=1 ./internal/system/` → **ok 8.102s**；`-count=3 ./internal/monitoring/...` exit 0。
- 仍未覆盖：栈级「调度器停摆后自愈」的端到端复现（需起栈，见 §9）；`-race` 下无检测（宿主无 cgo，见 §9-5）。

---

## 2. 12.2 契约一致性

### 2.1 实际调用（Java 公开面 `http://127.0.0.1:18080`，原始树）

`POST /api/v1/services` → 201；`GET /api/v1/services`、`GET/PATCH/DELETE /api/v1/services/{id}` → 200/200/200/204；
`GET /api/v1/services/{id}/timeline` → 200；`GET /api/v1/dashboard/summary` → 200；
`GET /api/v1/episodes`、`GET /api/v1/episodes/{id}`、`GET /api/v1/episodes/{id}/timeline` → 200；
`GET /api/v1/episodes/{id}/artifacts?type=INVESTIGATION|DIAGNOSIS_REPORT` → 200；
`retry-diagnosis`（已归档）→ 409、`retry-archive`（未恢复）→ 409；`GET /readyz` → 200。

### 2.2 偏差（原始树实测 → 修复后状态）

| # | 请求 | 期望 | 原始树实际 | 修复后状态 | 修复方式 / 证据 |
|---|---|---|---|---|---|
| 1 | `PUT /api/v1/services/{id}` | 405 + 统一 `{error_code,message}` | 405 但为 Spring 默认体 `{"timestamp","status","error","path"}` | **405 + 统一体**（单测覆盖，**未端到端复测**） | `PublicApiErrors.java`（§11-5）；`PublicApiErrorsTest.methodNotAllowedUsesUnifiedErrorBody` 通过（Java 20/0/0） |
| 2 | `GET /healthz` | 无此路由 | 404，Spring 默认体 | **404 + 统一体**（单测覆盖，**未端到端复测**） | 同处理器的 404 分支；`PublicApiErrorsTest.unknownRouteUsesUnifiedErrorBody` 通过 |
| 3 | 重复 `service_key` | 409 | **500 INTERNAL**（`uniq_services_service_key_active` SQLSTATE `23505`） | **409 `ALREADY_EXISTS`**（真实测试库验证，**未端到端复测**） | `error.go mapDBErr` + `store.go`（§11-1/2）；`TestCreateServiceDuplicateKeyIsConflict` 通过（0.31s）、`TestMapDBErrUniqueViolationTo409` 通过 |
| 4 | `retry-*` 对不存在 episode | 404 | **500 INTERNAL**（`sql: no rows in result set`） | **404 `EPISODE_NOT_FOUND`**（真实测试库验证，**未端到端复测**） | `task.go`（§11-3）；`TestRetryUnknownEpisodeIsNotFound` 通过（0.50s） |
| 5 | `>1.4 MiB` 请求体 | 413/400 且信息准确 | **400** `service_key, name and authority_producer_id are required` | **未复测** | 本轮未改动；保留原结论 |
| 6 | `artifacts` 不带 `type` | 400 | **200**，静默返回 INVESTIGATION；`type=bogus` 同样 200 | **未复测** | 本轮未改动；保留原结论 |
| 7 | `GET /api/v1/targets/{id}` | 契约只声明 PATCH/DELETE | 405 | **未变**（契约与实现一致，非缺陷） | — |
| 8 | 响应头 `X-Request-ID` | 透传 Go 的 request_id | **无** | **未复测** | 本轮未改动 |
| 9 | `POST /api/v1/services/{id}/targets` | 201 新增目标 | ~~405~~ | **接口存在，实测 201** | `internal/api/server.go:37`；**上一版报告此处为误判，已删除** |

> 说明：原 §2.1 的 19 条路由矩阵与 §2.2 的 #1–#4 结论均为**修复前的原始树**实测。两轮对 #1–#4 都只做到**单测/存储层验证**（第二轮虽起了两次 E2E 栈，但脚本不重放这些异常请求），未逐一重放 HTTP 请求，
> 因此记为「已改并有测试证据」，**不记为端到端通过**。#5/#6/#8 本轮未改动，保留原始树结论；#7 复核为契约与实现一致（非缺陷），保留；#9 为上一版误判，`internal/api/server.go:36-37` 上 GET/POST `/api/v1/services/{id}/targets` 均存在，已删除该“缺陷”。

### 2.3 契约文件与校验

- `contracts/openapi/public-api.v1.yaml` **只声明 path，无 requestBody/responses schema**，无法字段级校验；`retry-*` 声明 202，实测从不返回 202。
  本轮未修（仍存）。
- 全仓**无任何 OpenAPI 校验测试**（无 swagger/spectator/swagger-parser 依赖），也无 `tests/`、`scripts/`、CI 目录。

---

## 3. 12.3 端到端主链路

### 3.1 `deploy/e2e.sh`

原始树实测：已提交版脚本跑不起来（旧拓扑残留），逐条修正后 exit 0：

| 脚本缺陷 | 现象 | 修正 | 落盘状态 |
|---|---|---|---|
| 轮询 `${API_URL}/healthz`（Java 公开端口） | 60 次重试全失败 | 改为 `/readyz` | **已提交（`d55603c`）**：`:118`、`:123` |
| `docker compose exec -T agent ...` | `service "agent" is not running` | 改为 `agent-runtime` | **已提交（`d55603c`）**：`:126`、`:136`、`:355` |
| `FAULT_PORT` 只从 `deploy/compose/.env` 读，不传给 compose | `Bind for 127.0.0.1:18081 failed` | 运行时同时导出到 shell 环境 | 运行期操作，未改脚本 |
| 未注入口令变量 | `password authentication failed for user "alertflow"` | 启动前 source 环境文件 | 运行期操作，未改脚本 |

校验（第二轮脚本）：`bash -n deploy/e2e.sh` → exit 0；`ENV_FILE=deploy/compose/.env.example make compose-config` → exit 0（11 service）；
`grep -n 'exec -T agent \|/healthz' deploy/e2e.sh` → 无残留；`grep 'up -d --build' Makefile` → 无（`make e2e` 只调 `bash deploy/e2e.sh`）。

第二轮脚本改造（`d55603c`）：Makefile 的 `e2e` 只调用 `bash deploy/e2e.sh`；脚本自管栈生命周期——固定 `-p alertflow-e2e-<runid>`、
`up` 前注册唯一 `EXIT` trap、默认 `down -v --remove-orphans` 且只作用于本项目、`ALERTFLOW_E2E_KEEP_STACK=1` 保留现场且只打印清理命令、
失败时打 `dc ps` + 关键服务 `logs --tail=50`、启动前预检 `.env`/镜像变量文件/6 个必填口令非 `CHANGEME`/`compose config --quiet`。

**实际复跑结果（真实起栈，非模拟）**：

第二轮（指导执行轮；直接调用 `bash deploy/e2e.sh`）：

| 运行 | 命令 | 清理行为 | 结果 | 用时 |
|---|---|---|---|---|
| runA | `bash deploy/e2e.sh` | **默认清理** | exit 0：服务创建 → `/break` → Episode ACTIVE → 诊断 → 双产物（`INVESTIGATION v2` / `DIAGNOSIS_REPORT v2`）→ 恢复 → 归档（Episode `f65e8103-…`），退出时自动 `down -v` 清理 | `65.19 s` |
| runB | `ALERTFLOW_E2E_KEEP_STACK=1 bash deploy/e2e.sh` | **保留现场，不清理** | exit 0：同样验证到双产物（`INVESTIGATION v2` / `DIAGNOSIS_REPORT v2`）与归档（Episode `b85793a3-…`）；保留栈 project `alertflow-e2e-1794911809-76193`（9 容器 Up、`/readyz` 200），第二次启动无端口/卷冲突 | `47.32 s` |

第三轮（收尾整改；按指导直接跑 `make e2e`，两次都用默认清理）：

| 运行 | 命令 | 清理行为 | 结果 | 用时 |
|---|---|---|---|---|
| runA | `make e2e` | **默认清理** | exit 0：服务创建 → 故障注入 → Episode ACTIVE → 诊断（`DIAGNOSTIC_INFO` `input_version=2`）→ 双产物（`ARTIFACT_TYPE_INVESTIGATION v2` / `ARTIFACT_TYPE_DIAGNOSIS_REPORT v2`）→ 恢复 → 归档（Episode `e6137294-…`）；project `alertflow-e2e-1794914572-135182` 退出时自动清理，清理后 e2e 容器数 **0** | `61 s` |
| runB | `make e2e` | **默认清理** | exit 0：同样全链路（Episode `0cf8388b-…`）；project `alertflow-e2e-1794914636-141434` 自动清理，清理后 e2e 容器数 **0**；两次之间无端口/卷冲突 | `56 s` |

- 第三轮两次运行结束后，`docker ps -a` 中测试 project 的容器/网络/命名卷均为 0；其他 project 的容器（`og-redis`/`og-mysql`）与测试库容器 `af-test-pg` 全程未受影响（脚本清理只作用于 `-p alertflow-e2e-<runid>`）。
- **口径区分**：第二轮 runB 用了 `ALERTFLOW_E2E_KEEP_STACK=1`，**不算默认清理**；「两次都默认清理」这一结论由第三轮两次 `make e2e` 单独支撑。
- 第三轮首次运行（`make e2e` runA）暴露一个真实的就绪竞态并已修复（`460c6ab`）：postgres healthcheck 原来用 `pg_isready -U alertflow`（默认走 unix socket），而 initdb 阶段的临时实例只监听 socket，于是服务在最终 server 尚未监听 TCP 时就被判为 healthy，`db-init` 随即连 TCP 失败——
  `psql: error: connection to server at "postgres" (…), port 5432 failed: Connection refused` → `service "db-init" didn't complete successfully: exit 2`。
  修法：healthcheck 改为 `pg_isready -h 127.0.0.1 -U alertflow`（强制走 TCP），并在 `init-roles.sh` 首次 `psql` 前加入 30 次 × 1 s 的有界重试；修复后第三轮两次 `make e2e` 均未再出现该错误。
- 第二轮/第三轮 project name 各不相同（`alertflow-e2e-1794911744-69864` / `-1794911809-76193` / `-1794914572-135182` / `-1794914636-141434`），前一次清理后下一次未出现 `Bind for 127.0.0.1:* failed`。
- 第二轮首次跑通前遇到的三个真实问题：① 后台 WSL 进程 PATH 无 `make`（改用 `/home/sancho/.local/bin/make`，环境问题非代码）；② 本地 `eclipse-temurin:25-jdk` tag 实际指向 JRE（`No compiler is provided in this environment`）→ 用 daocloud 同名镜像重打 tag；③ **`db-init` 退出 2**：`deploy/compose/postgres/init-roles.sh` 在 `set -u` 下需要 `GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`，而 `db-init` 只传了 postgres 口令（481c217 起的回归）→ `91f5a51` 补齐两个 env 并让失败诊断带上 `db-init`。
- 第二轮 runB 保留的栈随后用于 §4.2a 的缓存接口级验证，验证完成后已 `down -v --remove-orphans` 清理，无残留容器与卷。
> 因此 §12.3 的「通过」对 `bash deploy/e2e.sh`（第二轮）与 `make e2e`（第三轮，两次均默认清理）都成立；第一轮那句「修复后脚本未再起栈」已作废。

通过轮（原始树，`COMPOSE_PROJECT_NAME=af-t1`、ScriptedModel、`FAULT_PORT=18085`）：服务 `fa587c56` 创建 → `POST /break` →
Episode `b77bb1d4` 1 个 ACTIVE → `DIAGNOSTIC_INFO` accepted（input_version 2）→ 双产物 v2
→ 打印「诊断模型: ScriptedModel（链路验证，非真实模型质量）」→ 恢复 → 归档。
事后核对：`agent_tasks` 5 条（4 DIAGNOSIS + 1 ARCHIVE）全部 SUCCEEDED，`attempt_count` 0，`last_error_code` 空；
`cognitive.episode_artifacts` 两类各 1 条。

### 3.2 手工补充链路（同栈，API 直连，原始树）

新建服务 + TCP 目标（`postgres:5432`）+ HTTP 目标（`fault/health`，阈值 2/2）：

1. `/break` → 目标 5 s 内 ABNORMAL；
2. **`is_required=false` 时服务聚合仍 NORMAL、不建 Episode**（`aggregateMonitor` 只看必需目标）——符合设计，页面需可解释；
3. `PATCH is_required=true` 后 6 s 内建 Episode，`started_by=MONITOR`；
4. 提交 `DIAGNOSTIC_INFO` → input_version 2 → 20 s 内双产物 v2（ScriptedModel 中文）；
5. `/recover` + `HEALTH_STATUS=NORMAL` → 10 s 内归档（`resolved_at 11:24:44` / `archived_at 11:24:45`，input_version 5）；
6. `GET /episodes/{id}/timeline` 返回 `episode`、`monitor_points(99)`、`monitor_start_point_seq`、`region1_projection`、`region2_events(2)`、`region3_refs(2)`。

> 注：第 1、3 步的「5 s / 6 s 内」恰好是 §5.3 P0 的漏网点 —— 该次运行调度正常，未触及停摆条件。

### 3.3 前端可达性（经 Nginx `13000`，原始树）

`/`、`/dashboard`、`/services`、`/services/{id}`、`/episodes`、`/episodes/{id}` 全部 200；
`/api/v1/episodes/{id}` 返回真实数据（`status=ARCHIVED`、中文 `service_name`）。未做浏览器点击级回归（§9）。

---

## 4. 12.4 版本缓存

### 4.1 正确行为（实测，原始树）

- 指针 key `af:cache:revision:<scope>:<id>`、值 key `af:api:<scope>:<id>:v<n>`；`PTTL` 5000–6000 ms 抖动。
- `FLUSHALL` 后首读重建值键与指针；`PATCH` 服务 → Go revision +1 → outbox → Kafka → Java `apply()` → Lua ADVANCE 推进指针。
- **乱序不回退**：隔离 `system-core`、手动 `set ptr=1000`，注入 `1100` → 指针 1100；再注入 `1005` → 仍 1100
  （`if current and tonumber(current) >= next then return 0`）。
- **坏消息容错**：`not-json`、缺 `scope`、`scope` 为空、`revision=-7` 四条注入后读取仍 200，指针未回退。
- **Redis 故障回退**：`docker stop af-t1-redis-1` 期间 `/api/v1/services` 200（0.042 s）、`/dashboard/summary` 200（0.026 s）；重启后恢复；`FLUSHALL` 后自行重建。

### 4.2 缺陷与修复状态

**(a) 读缓存恒不命中 —— 已修并经接口级验证**

原始树：`RevisionCache.around()` 命中要求进程内 `revisions` 已知且等于指针；该 map 只在 `apply()`（Kafka 消费）更新，
请求路径从不回填；未命中时用 `revisions.getOrDefault(key, 0L)` 写指针 → 冷启动等价「直连 Go + 一次 Redis 往返 + 一次 Lua」，
且把指针写成 0 而非 Go 真实 revision。进程外探针连续两次 `around()` 观测 `loader calls=2`（期望第二次命中）。

第一轮改法（`1c27947`，`services/integration/src/main/java/com/alertflow/integration/cache/RevisionCache.java`）：
命中判据改为「Redis 指针已知即命中」（新增 `pointers` 本地副本，`apply()` 时写）、写回改用 `refreshPointer()` 读真实指针。

第二轮改法（`3aba030` + `ab2b8dc`）——按指导文档把 query 纳入缓存键、删掉伪单飞，并修 Lua 缺陷：

1. 签名改 `around(scope, id, query, loader)`；值键 `af:api:<scope>:<id>:q<queryHash>:v<revision>`，
   `queryHash` = `SHA-256(query)` 前 16 hex，null/空/blank 归一为固定 `none`（**不把原始 query 拼进键**）；指针键 `af:cache:revision:<scope>:<id>` 不变；
2. **删除 `pins` 伪单飞**：`ConcurrentHashMap<String,Long>` 并不能合并并发回源（后来者会读到先到者尚未写入的值），
   删掉后允许并发回源；Redis 故障时 `redisGet`/`redisAdvance` 捕获 `RuntimeException` 降级直查 Go；
3. **修 Lua `ADVANCE` 缺陷（本轮接口级验证时发现）**：旧脚本只在指针前进时写值键，而值键含 query+revision →
   同一 revision 下只有第一个 query 能落缓存，其余 query 变体永远 miss；改为「指针前进才写指针、`next >= known` 即写值键」。

接口级证据（runB 保留栈，API `127.0.0.1:18080`，Redis 用该栈自己的容器）：

| 检查 | 操作 | 观测 |
|---|---|---|
| 不同 query 各自成键 | `GET /api/v1/services?limit=10`、`?limit=20` 后立即 `SCAN af:api:*` | 两键共存：`...:q9c21d9e643e1dcbf:v7`、`...:qa79faec4867b1f10:v7`（`sha256("limit=10")[:16]` = `9c21d9e643e1dcbf`） |
| 键内不含原始 query | 同上 | 只有 16 hex 摘要，无 `limit=10` 原文 |
| 同 query 命中 | 重复 `?limit=10` | body 相同、键数不增；值键 TTL 为 `5000 + rnd(0..999)` ms（5–6 s） |
| revision 推进后旧值不命中 | 新建服务 `cache-c-1794912206` → Kafka 消费 → 指针 v7→v8，再 `GET ?limit=10` | 响应**包含新服务**（旧 v7 结果未被使用），随后 `limit=10`/`limit=20` 均写 `...:v8` |
| 400 不写缓存 | `GET /api/v1/services?status=ACTIVE` | 400 `{"error_code":"VALIDATION","message":"status must be NORMAL, ABNORMAL or UNKNOWN"}`，无新键 |
| 单测 | `mvn test` | Tests run: 20, Failures 0, Errors 0，含 `RevisionCacheTest` 6 例 |

→ 本条升级为**已修并经接口级验证**（§8-B3）；仍未做：多副本一致性、并发击穿压测（§9）。

**(b) 写放大 —— 结论更正：普通采样 1 条，已按可见变化收窄**

旧版「每次采样两次 `noteViews`、各 5 个视图 → 一次采样 10 条」**不成立**：`apply.go:172` 与 `:194` 位于互斥分支
（`:172` 是 `if ep == nil { return s.noteViews(...) }`，命中即返回，不会再走到 `:194`）。原始树普通采样实际是 **5 条**
（一次 `noteViews` × 5 个视图）；原始树 9 分钟运行 557 个采样点 → outbox 2865 条 ≈ 557 × 5，与「5 条/采样」吻合，而不是 10 条。

第二轮受控计数（`revision_test.go`，断言「单次增量」：`resource_revisions` delta 恒为 1、outbox `RESOURCE_REVISION` 增量等于变化视图数）：

| # | 场景 | 单次 `RESOURCE_REVISION` 增量 |
|---|---|---|
| 1 | 普通采样（无 Episode） | **1**（仅 `targets:<id>`） |
| 2 | 已有 Episode 的普通采样（版本不变） | 1 |
| 3 | 首次 ABNORMAL 创建 Episode | **5**（`targets`、`service`、`services`、`episodes`、`dashboard` 五个视图均可见变化） |
| 4 | APP `HEALTH_STATUS` 改状态且版本变化 | 4 |
| 5 | 诊断任务完成但 Episode 不变 | 0 |
| 6 | 恢复采样 ABNORMAL→NORMAL（进入 resolution_pending） | **5**（同样是五个视图均可见变化，与 #3 是两个独立场景，不能合并计数） |
| 7 | 诊断完成 ACTIVE→RESOLVED | 3 |
| 8 | 归档 RESOLVED→ARCHIVED | 2 |

按上表顺序的 8 个计数为 **1/1/5/4/0/5/3/2**（其中两个 `5` 分别指「首次异常创建 Episode」与「恢复采样」；已与 `services/system-core/internal/system/revision_test.go` 的 `TestRevisionFirstAbnormalCreatesEpisode` / `TestRevisionRecoveryAndArchive` 核对）。

改法（`d2853fd`）：`revision.go` 新增 `viewChanges` 集合 + `noteViewChanges`（每视图至多 bump 一次），
`ApplyMonitorSample`/`ApplyAppEvent` 按真实可见变化选择视图；`task.go` 的 `CompleteTask` 补上此前**完全漏掉**的归档路径失效
（ACTIVE→RESOLVED 写 services+episodes+dashboard，RESOLVED→ARCHIVED 写 episodes+dashboard）。
元数据操作（store.go/console.go 6 处）仍走 `noteViews` 全量 5 视图；task 生命周期字段（`active_task_status`、`has_diagnosis_report`）
暂不触发 episodes/dashboard 失效，靠 5 s 缓存 TTL 兜底。本轮未重跑 9 分钟长跑统计 outbox 总量（§9）。

### 4.3 未执行

并发击穿**压测**（第二轮已删除伪单飞、改为允许并发回源，但**未做并发压测**）；多副本一致性（固定单副本）；9 分钟长跑 outbox 总量复测。

---

## 5. 12.5 运行与故障

### 5.1 依赖不可用（实测，原始树）

| 场景 | 操作 | 观测 |
|---|---|---|
| Go 不可用 | `docker pause` system-core | `GET /api/v1/services/{id}` → **504**，10.08 s，`{"error_code":"DEADLINE_EXCEEDED","message":"CallOptions deadline exceeded after 9.9997s"}` |
| Go 恢复 | `unpause` | 200，0.0165 s |
| Java 重启 | `docker restart` integration | `/readyz` 连接拒绝约 9 s，+12 s 恢复 200 |
| Redis 不可用 | `docker stop` redis | 读接口 200（回退查 Go） |
| PG 不可用 | `docker stop` postgres | `/readyz` → **503** `{"status":"unavailable","database":"down"}`；`/api/v1/services` → **500** `{"error_code":"INTERNAL"}` |
| PG 恢复 | `docker start` postgres | 8 s 内 `/readyz` 200 |

system-core 期间输出 `ERROR publish outbox`、`ERROR publish resource revision`、`ERROR apply monitor sample failed`：
**有日志、无指标**（全仓无 metrics；队列满/去重/超时均静默处理）。

### 5.2 关闭路径（代码核对）

`main.go`：`GracefulStop()` 后按 `ALERTFLOW_SHUTDOWN_TIMEOUT`（10 s）超时 `Stop()`；
`GrpcBridge`：`INTEGRATION_GRPC_SHUTDOWN_TIMEOUT_MS=15000` 后 `shutdownNow()`；
K8s `terminationGracePeriodSeconds`：system-core/integration/agent 45 s、frontend 15 s。在途请求场景未见截断告警。

### 5.3 **P0：监控调度永久停摆**

**现象（原始树，af-t1，2026-11-16）**：`docker pause system-core` 期间 Java 回调全部超时；`unpause` 后
`system.service_monitor_points` 永久停在 571，`last_check_at` 冻结，新建目标一直 UNKNOWN；
`docker restart system-core integration` 后仍 571→571。用本地编译二进制（`docker run debian:bookworm-slim .../system-core`，
同网络）复跑同样 571→571，**无 panic 输出**。

**根因（代码定位，三处叠加）**：

1. `internal/monitoring/scheduler/scheduler.go`（原始树）：`Sync` 在 `!item.inHeap` 分支重新 `push(item)`（约 `:89-94`），
   而 `PopDue` 之后 `inHeap=false` —— 5 s 一次的 `reconcile` 抢在 worker 的 `Defer` 之前把同一目标重新入堆；
   随后 worker 调 `Defer`，此时 `item.inHeap == true`，走 `heap.Fix(&s.items, item.index)`，**但 `item.index` 是 `push` 之后被
   `Swap`/`Push` 改写过的新下标，或指向 Sync 就地修改后的另一元素** → `heap.Fix` 不报错却把堆序改坏，
   `PopDue` 从此取不到最早到期元素或取到空 → **采样永久停止，且没有任何 panic**（与「无 panic 输出」的取证吻合）。
2. `internal/monitoring/worker`：每个 worker goroutine **无 `recover()`**，句柄函数一旦 panic 该 worker 静默死亡，4 个全死即永久停摆。
3. `internal/monitoring/engine.go:92-122`（原始树）：worker 回调里 `defer jobs.Done(...)` 与 `sched.Defer(...)` 不在同一锁序列内，
   且 `ctx` 取消后探测仍在跑却仍去 `Defer`。

**复现尝试（原始树，af-t2 全新栈）**：单目标、6 目标、交替 pause/unpause 均**未复现**（采样继续，24–28 条/25 s）。
故触发条件与 af-t1 历史（长时运行 + 大批在途探测 + 宿主睡眠）相关；本轮按代码路径定位并加确定性回归测试覆盖该竞态，而非等待自然复现。

**修复（`c9a5d50`，`services/system-core/internal/monitoring/`）**：

1. `scheduler/scheduler.go`：`Item` 增加 `Version`（配置变更 / 代次变更 / 出堆时递增）；`Defer(id, due, version)` 携带出堆时的快照版本，**版本不匹配即丢弃**（过期回推不再污染堆）；`Sync` 仅在「配置变化且不在堆中」时以 `now` 重新入堆，配置未变时对在途目标 `continue`（不再抢先重推）；新增 `syncLocked/pushLocked/removeLocked/popDueLocked/deferLocked`，把 `inHeap`/`index` 的维护收敛到这几处 —— `minHeap.Push` 置 `inHeap=true`、`Pop` 置 `inHeap=false`，`deferLocked` 先 `removeLocked` 再 `pushLocked`，**彻底消除在失效 `index` 上调用 `heap.Fix` 的路径**（根因 1）。
2. `worker/worker.go`：新增 `Options{OnPanic func(queue.Job, any)}`、`RunWithOptions(...)`、`safeHandle`（在单个 job 边界 `defer recover()`，恢复后继续消费；`Run` 退化为薄包装，保持旧签名）—— 根因 2。
3. `engine.go`：`jobs.Done(...)` 与 `sched.Defer(job.Target.TargetID, due, job.Version)` 收敛到同一处统一出口的 `defer`，`ctx.Err() != nil` 时不回堆；`OnPanic` 记录 `monitor probe panicked`；tick 循环改为 `TryEnqueue(queue.Job{..., Version: item.Version})` 传递版本 —— 根因 3。
4. `queue/queue.go`：仅新增 `Job.Version uint64` 快照字段，不改队列语义。

**修复后回归测试**：`go test -count=1 ./internal/monitoring/...` → **ok**（scheduler 0.197s / worker 0.020s / monitoring 0.193s / probe-http 0.285s / probe-kubernetes 0.020s）；`go test -count=3 ./internal/monitoring/...` → exit 0（不 flaky）。用例清单见 §1.5：`TestEngineNewTargetGetsFirstSample` 覆盖「新建目标不采样」，`TestWorkerSurvivesPanic`/`TestWorkerNeverDiesAcrossManyPanics` 覆盖 worker 静默死亡，`TestSchedulerConcurrentSyncPopDeferKeepsHeapOrder`/`TestSchedulerSyncRaceKeepsInvariants`/`TestDeferRejectsStaleVersion` 覆盖根因 1 的堆序竞态与版本失效。

**局限**：`-race` 不可用（宿主无 cgo：`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`），故本条按「**无竞态检测下的确定性回归测试**」记录；且**未在起栈后复现/验证「pause 后自愈」**（§9-10）。因此 P0 记为「**已修 + 单测证据**」，**不计为端到端通过**。

### 5.4 其它运行观测（原始树）

- `interval_ms=500` 的 6 个 TCP 目标实测 ≈450 ms/次（27 条/60 s）；`interval_ms=1000` 实测 ≈1.1 条/s —— 采样无堆积放大，符合预期。
- `PATCH interval 1000 → 3000` 后 5 s 内生效，转为 10 条/30 s，符合预期。
- `ALERTFLOW_MONITOR_*`（workers/queue size/timeout/reconcile）四个变量既未进 Compose 也未进 K8s 清单，只能靠默认值（**仍存**）。

---

## 6. 12.6 镜像与部署

### 6.1 国内源拉取（`docker.m.daocloud.io`）

| 镜像 | 原始树测试结果 | 修复后复测 |
|---|---|---|
| `library/golang:1.25.7-bookworm` | rc=0，4 s | 未复测 |
| `library/python:3.12-slim-bookworm` | rc=0，1 s | 未复测 |
| `library/node:22-alpine` | rc=0，1 s | 未复测 |
| `library/nginx:1.27-alpine` | rc=0，1 s | 未复测 |
| `library/debian:bookworm-slim` | rc=0，1 s | 未复测 |
| `library/eclipse-temurin:25-jdk` | **rc=124 超时**（90 s 层下载卡住） | **rc=0**：`docker pull docker.m.daocloud.io/library/eclipse-temurin:25-jdk` 成功（digest `sha256:884a2f…`）。注意本地已有部分层缓存，**耗时不可比**；结论是「可达」，不是「90 s 内可完成」 |
| `pgvector/pgvector:pg16`（manifest） | rc=0，10 s | 未复测 |
| `apache/kafka:3.9.1`（manifest） | rc=0，9 s | 未复测 |
| `nacos/nacos-server:v2.5.1`（manifest） | rc=0，25 s | 未复测 |
| `library/redis:7.4.2`（manifest） | **rc=124 超时**（96 s） | **rc=0**：`docker manifest inspect` 与 `docker pull -q` 均在 70 s 内完成 → 推翻「不可达」，改记为「可拉取，首次较慢」 |

- 修复后判定：**11 个镜像/清单全部可达**（`eclipse-temurin`、`redis` 两条 rc=124 复测为 rc=0）。但「可达」≠「快」：temurin 首次拉取曾 90 s 超时，CI/部署侧仍需配置 `docker.m.daocloud.io` 前缀或预拉取。
- **四个 Dockerfile 的 `FROM` 仍写 Docker Hub 上游名**，但变量已真正接入（`d55603c`）：`deploy/images.env.example` 从 5 个扩为 **11 个**（5 个运行镜像 + 6 个构建阶段镜像），Makefile 与 `deploy/e2e.sh` 都用 `--env-file $(IMAGES_ENV_FILE) --env-file $(ENV_FILE)` 传入；`compose.yaml` 的 5 个运行镜像改为 `${VAR:-原tag}`，4 个 Dockerfile 首个 `FROM` 改为 `ARG XXX=原tag` + `FROM ${XXX}`；E2E 两次真实 `--build` 均成功 → **「镜像变量无人读取」已修**。国内化前缀是否使用由部署侧在 `images.env` 里填，本轮未在国内源上重建全部镜像。
- 本地构建产物：system-core 197 MB、integration 662 MB、agent-runtime 535 MB、frontend 77.4 MB。

### 6.2 Compose

- 干净树 `make compose-config` exit 0；`docker compose config --quiet` 通过。
- **按 README 首条命令执行失败（P1，原始树）**：`cp deploy/compose/.env.example deploy/compose/.env` 后直接起栈，
  `postgres` 报 `Error: Database is uninitialized and superuser password is not specified.` 退出，整栈失败。
  模板首节 6 个口令（`POSTGRES_PASSWORD`/`GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`/`MCP_TOKEN`/`KNOWLEDGE_MCP_TOKEN`/`A2A_TOKEN`）为空且未标注「必须填写」。
- `.env.example` 第 2 节仍是旧拓扑命名（`alertflow:18080:8080`、`agent`、`INTEGRATION_*_GRPC_TARGET=alertflow:9090 / agent:9000`）；
  README `:74`/`:103` 让人「填写 `deploy/.env`」，实际是 `deploy/compose/.env`，`.gitignore:6` 原也只忽略 `deploy/.env`。
- 第一轮修复后状态（`f2a30b1`）：`.env.example` 6 个口令改为 `CHANGEME_*` 占位并标「必须修改」（`POSTGRES_PASSWORD`/`GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`/`MAIN_MCP_TOKEN`/`KNOWLEDGE_MCP_TOKEN`/`A2A_TOKEN`，`:30/:33/:36/:39/:42/:45`）；头部用法改为带 `--env-file deploy/compose/.env`；旧拓扑名 `alertflow`→`system-core`、`agent`→`agent-runtime`（`:64/:118/:159`）。README `:73-77` 首条命令补 `--env-file deploy/compose/.env` 与「必须修改口令」提示，`:74`/`:103` 路径改为 `deploy/compose/.env`，`:88` `down` 同样补 `--env-file`。
- 验证边界（第一轮）：`ENV_FILE=deploy/compose/.env.example make compose-config` → **exit 0**，且渲染结果中 6 个口令均非空，第一轮只做到渲染层。
- **第二轮已实测起栈**：`cp .env.example .env` 后改 6 个口令，真实起栈两次均 exit 0（§3.1；第二次用 `ALERTFLOW_E2E_KEEP_STACK=1` 保留现场）。首次起栈暴露一个真实回归并已修（`91f5a51`）：`deploy/compose/postgres/init-roles.sh` 在 `set -u` 下需要 `GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`（481c217 把硬编码口令改成变量却漏了给 `db-init` 传这两个 env），导致 `db-init` 以 `GATEWAY_PASSWORD: parameter not set` 退出 2、整栈失败；现已在 `db-init.environment` 补传，且 `e2e.sh` 失败诊断会打印 `db-init` 日志。
- **第三轮复测（收尾）**：用 `make e2e` 以默认清理连跑两次，均 exit 0（61 s / 56 s）；期间首次运行因 postgres 就绪竞态失败（`db-init` 连接被拒），已由 `460c6ab` 修复（healthcheck 强制走 TCP + `init-roles.sh` 有界重试），修复后两次均通过。
- 因此 §8-B1 从「已改未实测」升级为「**已修并实测**」。

### 6.3 Kubernetes

- 本机 `kubectl` context `kind-alertflow-lab` 指向 `127.0.0.1:57026` **连接被拒**，无可用集群 → **未部署、未启动验证**，仅静态校验（§9）。

| 项 | 原始树静态校验结果 | 修复后 |
|---|---|---|
| `00-namespace` 2 文档（Namespace + SA `alertflow-go`） | 通过 | — |
| `10-rbac` 2 文档（Role 仅 `apps` 的 `deployments/statefulsets` + `get`，绑定 `alertflow-go`；**无 ClusterRole**） | 通过 | — |
| `20-configmap` 3 文档；所有 Deployment 的 `configMapRef` 均可命中 | 通过（但 `ALERTFLOW_GRPC_ADDR: ":9090"` **重复两次**） | **已改**：重复项删除，`grep -n ALERTFLOW_GRPC_ADDR` 仅剩 `:21` 一行（`:9090`）；integration-config 补 `INTEGRATION_REDIS_URL`/`INTEGRATION_KAFKA_BROKERS` |
| `30-services` 4 文档：system-core 9090 / integration 8081+9091 / agent 8000+9000 / frontend 80 | 通过 | — |
| Deployment 4 个、replicas=1、`strategy: Recreate` | 通过（无 HPA、无多副本） | — |
| Secret `alertflow-secrets` 与各 `secretKeyRef` | 引用一致；`secret.example.yaml` 头部仍写「Go Backend」「`deploy/k8s/`」 | **部分改**：`secret.example.yaml:9` 的 `kubectl apply -f deploy/k8s/` → `deploy/kubernetes/base/`；「Go Backend」作为组件统称**保留未改**（P2，§8-C） |
| 镜像 | **旧名** `ghcr.io/ecakeman/alertflow-{backend,integration,agent,frontend}:v0.1.0` | **已改**：`40-deployments.yaml:42` → `ghcr.io/ecakeman/alertflow-system-core:v0.1.0`、`:233` → `ghcr.io/ecakeman/alertflow-agent-runtime:v0.1.0`（integration/frontend 原名未变）；`deploy/kubernetes/README.md` 镜像名表同步 |
| 探针 | integration `/actuator/health/liveness\|readiness`（本机实测 200）、agent `/health/live\|ready`、frontend `/healthz`（本机实测 200）；**system-core 的 liveness 与 readiness 同为 `exec ["system-core","health"]`** | **未拆分（本轮有意不做）**：核实 `services/system-core/cmd/system-core/main.go:29-30` 只有无参 `health` 子命令（`healthCheck` → gRPC health `Check` → `systemapi/server.go:31` → `:24-27` 仅 `Store.DB.PingContext`），**不存在按 readiness 区分的 flag/参数**；臆造拆分会让 liveness 一并失真，故只在 `:69-72` 加语义注释，记为 P2 未修 |
| Dockerfile 非 root | system-core `USER 10001`、integration `USER 10002`、agent `USER 10001`；frontend 标准 Nginx（root master 绑 80，清单未设 `runAsNonRoot`） | — |
| `deploy/kubernetes/README.md` | 仍写 `/api/` → `http://alertflow:8080`（实际 `integration:8081`）、旧镜像名 | **已改**：`:70/:169/:240` 代理目标 → `integration:8081`；`:7/:9/:48/:49/:58/:59` 镜像名同步；`:92/:109/:124-127/:130` 路径 `deploy/k8s/` → `deploy/kubernetes/base/`；`:71` 镜像要求改 system-core |

- **第二轮进一步对齐（`f4e5707`）**：`deploy/kubernetes/README.md` 组件表补 Service/Deployment 列；Go 侧 HTTP 改为「无/不对外提供」+ gRPC `9090`；Redis 全部改由 Java 侧 `INTEGRATION_REDIS_URL` 承担；`alertflow` 命名 → `system-core`（`:23/:145/:191`）；探针表 Go 改为 `exec system-core health`（`:158-163`）；Secret 示例命令删除 `ALERTFLOW_REDIS_URL`（`:97-99/:119`）；Frontend 的 `location = /healthz` 标注「与 Go 无关」（`:72/:161`）。`20-configmap.yaml:8/:58-59`、`secret.example.yaml:11-14/:39-43` 同步。README 中 `8080` 已清零，`healthz/readyz` 仅剩 2 处 Frontend。

静态校验（两轮，工作区/已提交版）：6 个 YAML（`00`/`10`/`20`/`30`/`40` + `secret.example`）`pyyaml.safe_load_all` 全部解析成功（文档数 2/2/3/4/4/1）；残留 grep（`deploy/k8s`、`alertflow-backend`、`agent/src/alertflow_agent/http_app`、`alertflow:8080`、README 中 `8080`）无匹配。**未部署、未 `kubectl apply`，K8s 侧仍只算静态校验。**

---

## 7. 文档与代码一致性核对（原始树实测）

| 位置 | 问题 | 修复后 |
|---|---|---|
| `README.md:73-75` | `cp deploy/compose/.env.example deploy/compose/.env` + `docker compose ... up -d --build` 在空口令模板下**直接失败**（§6.2） | **已改**：`:73-77` 首条命令补 `--env-file deploy/compose/.env` 与「必须修改口令」提示（§6.2 已渲染验证；未 `up -d`） |
| `README.md:74`、`:103` | 「填写 `deploy/.env`」，该文件不存在；实际是 `deploy/compose/.env` | **已改**：两处路径改为 `deploy/compose/.env` |
| `README.md:142` | 链接 `deploy/k8s/README.md`，**路径不存在**（实际 `deploy/kubernetes/README.md`） | **已改** |
| `README.md:49-52` | 维护表仍写 `frontend/`、`backend/`、`agent/` 旧目录 | **已改**：改为 `apps/console`、`services/system-core`、`services/integration`、`services/agent-runtime` |
| `deploy/compose/.env.example:4-6` | 用法注释写 `cp deploy/.env.example deploy/.env`、`deploy/docker-compose.yml`（均不存在） | **已改**：头部用法重写（`cp` + `--env-file`），旧服务名 → `system-core`/`agent-runtime` |
| `docs/engineering/runtime-and-http.md:93` | `agent/src/alertflow_agent/http_app.py` 旧路径 | **已改**：→ `services/agent-runtime/src/alertflow_agent/http_app.py` |
| `docs/design/v1.md:1658/1667` | `proto/alertflow/agent/v1/agent.proto`、`gen/agent/v1` 旧路径 | **已改**：`:1658` → `contracts/proto/alertflow/agent/v1/agent_execution.proto` 与 `agent_system_gateway.proto`；`:1667` `go_package` → `services/system-core/gen/agent/v1;agentv1` |
| `apps/console/src/api/client.ts:4/31` | 注释仍写 "Go Backend" | **已改**：两处 → "Java Integration" |

> 本节全部为静态文档核对。第一轮 10 个文件 +72/−58，只做 YAML 解析、`bash -n`、`make compose-config`、残留 grep 四类校验；
> 第二轮又改了 `deploy/kubernetes/README.md`、`20-configmap.yaml`、`secret.example.yaml`（`f4e5707`，K8s 侧见 §6.3）。

---

## 8. 阻塞项与修复状态

> 状态取值：**已修**（本轮改动 + 有实验证据）、**已改未复测**（本轮改动但无端到端复测）、**仍存**（本轮未改）、**未复测**（未改动也未再验证）。
> 不带证据的「已修」不成立：每条「已修/已改」都必须附本轮实测命令或测试名（见 §1.2、§1.5、§11 的验证列），否则一律记为「未复测」或「仍存」。
> - 第一轮提交：`c9a5d50`、`1c27947`、`39d8140`、`f2a30b1`（已提交并推送）；
> - 第二轮代码提交：`d55603c`、`3aba030`、`d2853fd`、`f4e5707`、`91f5a51`、`ab2b8dc`（已提交并推送）；
> - 第三轮代码提交：`460c6ab`（已提交并推送）；
> - 被测代码基线：`460c6ab`；报告历史节点：首次入库 `7af5573`、后续修订 `83727c9`、README 与报告对齐 `5311548`。当前 `main` HEAD 以 `git rev-parse origin/main` / GitHub 分支页面为准，不在此处硬编码。

### A. P0 —— 监控调度停摆

- 状态：**已修**（`c9a5d50`；有确定性回归测试，**无栈级复现** → 不记为端到端通过）
- 根因与修法见 §5.3；10 个监控回归用例见 §1.2 与 §11。

### B. P1 —— 交付可用性

1. **README 首条命令失败** —— **已修并实测**：第二轮用自己的 `.env` 连跑两次 E2E 均 exit 0；第三轮按收尾指导用 `make e2e` **默认清理连跑两次**均 exit 0（61 s / 56 s，§3.1、§6.2）。期间还暴露并修掉两个部署层问题：`db-init` 缺 `GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD` 导致整栈失败（`91f5a51`），以及 postgres healthcheck 走 socket 造成的就绪竞态（`460c6ab`）。
2. **~~目标管理缺 Go 接口~~** —— **删除**。`POST /api/v1/services/{service_id}/targets` 存在（`internal/api/server.go:36-37`，实测 201）。上一版为误判。
3. **`RevisionCache` 读路径恒不命中** —— **已修并经接口级验证**（`1c27947` + `3aba030` + `ab2b8dc`，§4.2a）：不同 query 各自成键、同 query 命中、revision 推进后旧结果不命中、Redis 故障回源；**并发击穿压测与多副本仍未做**（§9）。
4. **写放大** —— **结论更正 + 已收窄**（`d2853fd`，§4.2b）：普通采样 **1** 条（旧版「10 条」是把两个互斥 `noteViews` 调用点重复计数，原始树实际 5 条），首次异常/恢复采样 5 条，元数据操作仍 5 条；**仍存**：元数据路径仍全量 5 视图、task 生命周期字段不触发 episodes/dashboard 失效（靠 5 s TTL 兜底）。
5. **错误映射** —— 保持上轮结论：Go `23505 → 409`、`ErrNoRows → 404` 有单测与真实测试库用例；Java 405/404 统一体已改（`PublicApiErrorsTest` 3 例）；`artifacts` 缺 `type` 仍返回 200（未改）。
6. **K8s 文档与清单不一致** —— **第二轮已对齐**（`f4e5707`，§6.3）：README 组件表/端口/探针/Secret 命令、`20-configmap`、`secret.example` 全部同步，6 个 YAML 解析通过、README 中 `8080` 清零；**仍未在真实集群部署**（§9）；探针未拆分（P2）、`secret.example` 的「Go Backend」字样保留（P2）。

### C. P2 —— 工程质量

- **gofmt**：剩 **2** 个文件（`services/system-core/cmd/system-core/artifacts.go`、`internal/gateway/server.go`，均为 `d763fd1` 既存）；`internal/system/task.go` 已随 `d2853fd` 格式化。
- **Python wheel** —— **已修**（`f4e5707`）：`uv build` 产出 `alertflow_agent-0.1.0-py3-none-any.whl` **30,273 B / 20 个模块**，`pip install` 后 `import alertflow_agent` 成功；无 `ruff` 仍存。
- **测试覆盖**：`scheduler`/`worker`/`engine` 已补（10 用例），`queue`/`probe/tcp`/`gateway`/`transport`/`db` 仍无测试文件；Java 19 条公开路由仍无控制器层测试；OpenAPI 校验未接入 —— **仍存**。
- 全仓无 metrics；`ALERTFLOW_MONITOR_*` 未进清单 —— **仍存**。
- `-race` —— **未执行**（宿主无 gcc，`-race requires cgo`）。

### D. 第二轮验证中发现并修复的两个新问题

1. **`db-init` 退出 2（`91f5a51`）**：`deploy/compose/postgres/init-roles.sh` 在 `set -u` 下需要 `GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`，而 481c217 把硬编码口令改成变量后漏了给 `db-init` 传这两个 env → `GATEWAY_PASSWORD: parameter not set`，整栈启动失败。已补 env 并让 `e2e.sh` 失败诊断打印 `db-init` 日志。
2. **缓存 Lua `ADVANCE` 只在前移时写值键（`ab2b8dc`）**：值键含 query+revision，旧脚本导致同一 revision 下只有第一个 query 能落缓存、其余 query 变体永远 miss。改为「指针前移才写指针、`next >= known` 即写值键」；修后接口级复验两 query 键共存、同 query 命中、v8 推进后旧值不命中（§4.2a）。

---

### E. 指导文档第 10 节完成判定（逐条对照）

| 判定条件 | 结果 | 证据 |
|---|---|---|
| `make e2e` 不再由 Makefile 与脚本各起一套 Compose project | 满足 | `Makefile:28-30` 只留 `bash deploy/e2e.sh`；脚本自建 `-p alertflow-e2e-<runid>`（`d55603c`，§3.1） |
| E2E 默认结束清理自己的容器/网络/卷，不影响开发环境 | 满足 | runA 退出打印清理本项目并实际 `down -v --remove-orphans`（仅限 `-p`）；`og-redis`/`og-mysql` 全程未受影响（§3.1） |
| 两次连续 E2E 均 exit 0，且都验证到 `ARCHIVED` 与两个非空产物 | 满足 | runA `f65e8103-…`、runB `b85793a3-…`，两次都有 `INVESTIGATION v2` + `DIAGNOSIS_REPORT v2`（§3.1） |
| 不同 query 不共用同一个 Redis 结果缓存键 | 满足 | `...:q9c21d9e643e1dcbf:v7` 与 `...:qa79faec4867b1f10:v7` 共存（§4.2a） |
| revision 增加后旧 query 结果不再命中 | 满足 | 新建服务 → 指针 v7→v8 → `?limit=10` 响应含新服务，随后写入 `...:v8`（§4.2a） |
| Go 失效范围按真实数据变化，且有测试验证单次写入数量 | 满足 | `revision_test.go` 8 类场景计数 **1/1/5/4/0/5/3/2**（两个 `5` = 首次异常创建 Episode、恢复采样；§4.2b、`d2853fd`） |
| K8s README 与 YAML 的端口/探针/资源名/Redis 归属一致 | 满足 | `f4e5707`；6 YAML `safe_load_all` 通过、README `8080` 清零（§6.3） |
| 验收报告版本边界可追溯且不自我引用 | 满足 | 被测代码基线 `460c6ab`；报告首次入库 `7af5573`；后续报告修订 `83727c9`；README 与验收报告对齐 `5311548`。这些是固定历史节点；当前 `main` HEAD 以远端 ref 为准，不在报告中硬编码 |

> 说明：判定之外的项（真实 K8s 集群部署、真实 LLM、`-race`、pause 自愈的栈级复现、契约 HTTP 重放、并发压测）仍未执行，见 §9，**不计为通过**。

---

## 9. 未执行 / 未验证（不计为通过）

1. **真实 K8s 部署与启动验证**（`kubectl` context `127.0.0.1:57026` 连接被拒，无可用集群）。
2. **真实模型诊断**（未设置 `ALERTFLOW_LLM_API_KEY`，第二轮 E2E 全程 ScriptedModel）；Knowledge Agent 未启用。
3. `make test` 全量目标（等价组件已分别验证，未整体执行）。
4. 浏览器点击级回归（仅 HTTP 可达性与历史截图）。
5. Go `-race`（宿主无 gcc，`-race requires cgo`）。
6. **并发压测**：缓存击穿、写放大容量影响只做计数观测，未做并发/容量压测。
7. Java 19 条公开路由的控制器层测试、OpenAPI 契约校验（仓内不存在）。
8. 多副本一致性（Compose 固定单副本、K8s replicas=1）。
9. **契约 #1–#4 的 HTTP 端到端重放**（只有单测/真实测试库证据，未在栈上逐条重放请求）。
10. **P0「pause 后自愈」的栈级复现**（未重跑该场景，只有确定性单测）。
11. **9 分钟长跑 outbox 总量复测**（写放大收窄后的绝对值未重新统计）。
12. **国内源重建全部镜像**（只验证拉取可达；E2E 用的是本机构建缓存）。
13. **CI**：仓库没有 CI 流水线，本报告全部结果是本地执行，**不记作 CI 通过**。

> 第二轮已完成、因此从本清单移除的旧项：`deploy/e2e.sh` 修复版实跑（现连续两次 exit 0）、`RevisionCache` 接口级验证、按 README 起栈（`make e2e`）、镜像 `--build` 重建、Python wheel 打包、K8s 文档与 YAML 对齐。
>
> 第三轮（收尾）只补测「`make e2e` 两次默认清理」并修复 postgres 就绪竞态（`460c6ab`），未改变上述任何未执行项的结论。

---

## 10. 采样清单（可复现命令与产物）

- Go：`cd services/system-core && export PATH=/home/sancho/.local/go/bin:$PATH && gofmt -l . && go vet ./... && go build ./... && go test -count=1 ./...`
- Java：`JAVA_HOME=/home/sancho/tools/jdk-25.0.5+8 /home/sancho/tools/apache-maven-3.9.11/bin/mvn -B -s services/integration/maven-settings.xml -Dmaven.repo.local=/home/sancho/tools/m2repo -f services/integration/pom.xml test`
- Python：`cd services/agent-runtime && uv sync --frozen && uv run pytest -q && uv build`
- Vue：`cd apps/console && npx vue-tsc --noEmit && npm run build`
- Compose：`ENV_FILE=deploy/compose/.env.example make compose-config`；**`make` 不在后台 WSL 的 PATH 里，需写 `/home/sancho/.local/bin/make`**
- E2E（第二轮）：`cp deploy/compose/.env.example deploy/compose/.env` 并改 6 个 `CHANGEME_*` 口令 → `bash deploy/e2e.sh`（默认清理；第二次用 `ALERTFLOW_E2E_KEEP_STACK=1` 保留现场）；日志 `/tmp/e2e-run-a.log`、`/tmp/e2e-run-b.log`、汇总 `/tmp/e2e-twice.out`
- E2E（第三轮，收尾）：`/home/sancho/.local/bin/make e2e` **连跑两次，均默认清理**；日志 `/tmp/e2e-final-a.log`、`/tmp/e2e-final-b.log`、汇总 `/tmp/e2e-final-wrapper.log`（脚本 `/tmp/e2e-final-twice.sh`）
- 缓存接口级检查（保留栈）：`curl -s 'http://127.0.0.1:18080/api/v1/services?limit=10'` / `?limit=20` 后立即 `docker exec <runB>-redis-1 redis-cli --scan --pattern 'af:api:*'`；revision 推进用 `POST /api/v1/services` 建服务，再从 Kafka 消费日志确认指针 v7→v8
- 监控复现脚本：`/tmp/mon-repro.sh`；P0 现场记录 `/home/sancho/af-monitor-repro.md`
- 契约矩阵：`/tmp/api_matrix.sh`（Java 公开面 19 条路由）
- 验证日志：第一轮 `/tmp/lead-go.log`、`/tmp/lead-java.log`、`/tmp/lead-py.log`、`/tmp/lead-vue.log`、`/tmp/lead-compose.log`（脚本 `/tmp/lead-validate.sh`）；第二轮 `/tmp/e2e-run-a.log`、`/tmp/e2e-run-b.log`、`/tmp/rebuild-integration.log`；早期 b1 日志 `/tmp/b1-*.log`（当时工作树被并发改动，仅作参考）；第三轮（收尾）全量校验日志 `/tmp/verify-final.log`（脚本 `/tmp/verify-final.sh`）

---

## 11. 三轮整改与分支整理记录

> - **第一轮**（`d763fd1` → `f2a30b1`）：30 项整改以 4 批提交入库 —— `c9a5d50`（system-core 监控与错误映射）、`1c27947`（integration 统一错误体 + 缓存指针）、`39d8140`（console echarts 按需引入）、`f2a30b1`（deploy/docs 对齐 + 本报告初版）。详见 11.1。
> - **第二轮**（`f2a30b1` → `ab2b8dc`）：按《AlertFlow整改执行指导-2026-11-17》的 6 个代码提交 —— `d55603c`、`3aba030`、`d2853fd`、`f4e5707`、`91f5a51`、`ab2b8dc`。详见 11.2。
> - **第三轮**（`ab2b8dc` → `460c6ab` + 文档收尾）：`460c6ab`（postgres TCP 就绪竞态修复）、验收报告历史修订节点（`7af5573`、`83727c9`）以及 README/报告对齐提交 `5311548`；远端 5 个维护分支 fast-forward 同步。`5311548` 只修改 README 与验收报告，不含业务逻辑或部署代码变更。详见 11.3。
> 「验证」列只写实际执行过的检查。

### 11.1 第一轮（`d763fd1` → `f2a30b1`）

| # | 文件 | 改动 | 验证 |
|---|---|---|---|
| 1 | `services/system-core/internal/system/error.go` | 新增 `mapDBErr(err, code, message)`：pgx `pgconn.PgError` `23505` → 409；`IsCode` 改用 `errors.As` | `go vet`/`go build` exit 0；`TestMapDBErrUniqueViolationTo409`、`TestMapDBErrPassesThroughOthers` 通过（`internal/system ok 5.778s`） |
| 2 | `services/system-core/internal/system/store.go` | `CreateService` 的 INSERT 错误走 `mapDBErr(err,"ALREADY_EXISTS","service_key already exists")`（重复 key 500→409） | `internal/system ok 5.778s`；`TestCreateServiceDuplicateKeyIsConflict` 通过（0.31 s，打真实测试库） |
| 3 | `services/system-core/internal/system/task.go` | `RetryDiagnosis`/`RetryArchive` 的 `sql.ErrNoRows` → 404 `EPISODE_NOT_FOUND` | `TestRetryUnknownEpisodeIsNotFound` 通过（0.50 s，打真实测试库） |
| 4 | `services/system-core/internal/system/apply_test.go` | `TestPublishPendingMarksSent` 按新语义重写（`PublishPending` 只发 `AGENT_TASK_READY`，`RESOURCE_REVISION` 走 `PublishByType`） | `internal/system ok 5.778s`，该用例通过 |
| 5 | `services/system-core/internal/system/error_test.go`（新） | `mapDBErr` 两个单测 | 通过（`internal/system`） |
| 6 | `services/system-core/internal/system/contract_error_test.go`（新） | 重复 `service_key`→409、未知 episode→404 两个**真实测试库**契约用例 | 通过（0.31 s / 0.50 s） |
| 7 | `services/integration/.../api/PublicApiErrors.java`（新） | `@RestControllerAdvice extends ResponseEntityExceptionHandler`，把 Spring 默认 404/405/400 体 `{"timestamp","status","error","path"}` 统一成 `{error_code,message}` | `mvn test` BUILD SUCCESS |
| 8 | `services/integration/.../PublicApiErrorsTest.java`（新） | 405 / 404 / POST 正常 3 用例 | `Tests run: 14, Failures: 0, Errors: 0`，其中该测试类 3/3 通过（`methodNotAllowedUsesUnifiedErrorBody`、`unknownRouteUsesUnifiedErrorBody`、`readableBodyStillAnsweredWithJson`） |
| 9 | `services/integration/.../cache/RevisionCache.java` | 读缓存：`pointers` 指针本地副本 + `pins` 合并回源 + `around()` 改判 `cachedPointer(key)==pinned` + 写回走 `refreshPointer()` | 编译与 `mvn test` 通过；**无该行为测试、无端到端证据**（§4.2a） |
| 10 | `services/system-core/internal/monitoring/scheduler/scheduler.go` | `Item.Version` + 版本化 `Defer` + 锁内 `push/remove/pop/defer` + 消除失效 `index` 上的 `heap.Fix` | `go test ./internal/monitoring/scheduler/` → ok 0.197 s（5 用例），`-count=3` exit 0 |
| 11 | `services/system-core/internal/monitoring/scheduler/scheduler_test.go`（新） | 堆序/竞态/版本失效 5 用例（含 `requireHeapOrder`、`countInHeap`） | ok 0.197 s |
| 12 | `services/system-core/internal/monitoring/worker/worker.go` | `Options{OnPanic}` + `RunWithOptions` + `safeHandle`（单 job 边界 recover），`Run` 薄包装 | `go test ./internal/monitoring/worker/` → ok 0.020 s（3 用例） |
| 13 | `services/system-core/internal/monitoring/worker/worker_test.go`（新） | panic 存活 / 多次 panic / 旧入口仍恢复 3 用例 | ok 0.020 s |
| 14 | `services/system-core/internal/monitoring/engine.go` | 统一出口 `defer jobs.Done` + `defer sched.Defer(..., job.Version)`；`ctx.Err()` 不回堆；`OnPanic` 记 `monitor probe panicked`；tick 传 `Version` | `internal/monitoring` ok 0.193 s（含引擎 2 用例） |
| 15 | `services/system-core/internal/monitoring/engine_test.go` | 新增首采样、panic 存活 2 用例 | ok 0.193 s |
| 16 | `services/system-core/internal/monitoring/queue/queue.go` | 新增 `Job.Version uint64` 快照字段（不改队列语义） | 编译与全量测试通过 |
| 17 | `apps/console/src/views/DashboardView.vue` | echarts 按需引入（BarChart + Grid/Legend/Tooltip + CanvasRenderer） | `npm run build` exit 0；`DashboardView-BaeayJ-X.js` 462,991 B（gzip 155.81 kB，原 1043 kB） |
| 18 | `apps/console/vite.config.ts` | **未生效**：仍为 `chunkSizeWarningLimit: 1600`（`vite.config.ts:18`），改动已丢失，未重做 | `grep -n chunkSizeWarningLimit` |
| 19 | `deploy/kubernetes/base/20-configmap.yaml` | 删重复 `ALERTFLOW_GRPC_ADDR: ":9090"`；integration-config 补 `INTEGRATION_REDIS_URL`、`INTEGRATION_KAFKA_BROKERS` | pyyaml 解析通过；`grep -n ALERTFLOW_GRPC_ADDR` 仅 `:21` |
| 20 | `deploy/kubernetes/base/40-deployments.yaml` | 镜像名 `alertflow-backend/agent` → `alertflow-system-core/agent-runtime`；`:69-72` 探针加语义注释（**未拆分**） | pyyaml 解析通过；探针未拆分的理由见 §6.3 |
| 21 | `deploy/kubernetes/base/secret.example.yaml` | `:9` `kubectl apply -f deploy/k8s/` → `deploy/kubernetes/base/` | pyyaml 解析通过；「Go Backend」字样保留 |
| 22 | `deploy/kubernetes/README.md` | 代理目标→`integration:8081`、镜像名表、路径→`deploy/kubernetes/base/` | 残留 grep 无匹配 |
| 23 | `README.md` | `:49-52` 目录、`:73-77` `--env-file`+改口令提示、`:74`/`:103` 路径、`:88` down、`:142` k8s README 路径 | 残留 grep 无匹配 |
| 24 | `docs/engineering/runtime-and-http.md`、`docs/design/v1.md`、`apps/console/src/api/client.ts` | 旧路径/旧包名/"Go Backend" 字样 | 残留 grep 无匹配 |
| 25 | `deploy/compose/.env.example` | 头部用法重写；6 口令标 `[必填]` 改 `CHANGEME_*`；旧服务名 `alertflow:9090`→`system-core:9090`、`agent:9000`→`agent-runtime:9000` | `ENV_FILE=deploy/compose/.env.example make compose-config` exit 0，渲染口令非空 |
| 26 | `Makefile` | 新增 `ENV_FILE ?= deploy/compose/.env`，compose-up/down/e2e 带 `--env-file` | `make compose-config` exit 0 |
| 27 | `.gitignore` | 新增 `deploy/compose/.env` | `git status --short` 不再显示该文件 |
| 28 | `deploy/e2e.sh` | `:118/:123` `/healthz`→`/readyz`；`:126/:136/:355` `exec -T agent`→`agent-runtime` | `bash -n` exit 0；`grep` 无残留；第二轮已被 `d55603c` 整体重写并由 `make e2e` 两次 exit 0 验证（§3.1） |
| 29 | `docs/operations/acceptance-2026-11-17.md`（本文件，新） | 本报告 | — |

合计：**23 modified + 7 untracked = 30 项**（提交前 `git status --short`），其中新增测试文件 4 个、新增 Java 文件 2 个；已全部随上述 4 个提交入库。

其他事实：

- Vue 主包 `index-Com4i6yR.js` = **1,219.00 kB**（`gzip 392.30 kB`；element-plus 全量未拆分，**仍存**）。
- 写放大：第一轮对 `noteViews` 未动；第二轮已按可见变化收窄（§4.2b、下表 D-3）。
- 第一轮提交完成后 `origin/main` = `f2a30b1`。

### 11.2 第二轮（`f2a30b1` → `ab2b8dc`，按《AlertFlow整改执行指导-2026-11-17》）

| # | 提交 | 文件 | 改动 | 验证 |
|---|---|---|---|---|
| D-1 | `d55603c` fix(deploy) | `Makefile`、`deploy/e2e.sh`、`deploy/compose/compose.yaml`、`deploy/images.env.example`、4 个 Dockerfile | 单一入口 `make e2e` → `bash deploy/e2e.sh`；脚本自管栈生命周期（唯一 EXIT trap、默认 `down -v --remove-orphans` 限本项目、`ALERTFLOW_E2E_KEEP_STACK=1` 保留现场、失败打印 `ps`+日志、启动前预检 `.env`/口令/`compose config`）；镜像变量接入（11 变量 + `${VAR:-tag}` + `ARG`） | `bash -n` 0；`make compose-config` 0（11 service）；第二轮两次真实起栈 exit 0（65.19 s / 47.32 s，第二次用 `KEEP_STACK` 保留现场）；无端口冲突、无残留 |
| D-2 | `3aba030` fix(integration) | `RevisionCache.java`、`PublicApiController.java` | `around(scope,id,query,loader)`；值键 `q<sha256[:16]>:v<rev>`；空 query → `none`；删 `pins` 伪单飞；Redis 故障降级；5 个调用点传入 `request.getQueryString()` | `mvn test` 20/0/0；接口级：两 query 各成键、同 query 命中、键不含原始 query |
| D-3 | `d2853fd` fix(system-core) | `revision.go`、`apply.go`、`task.go`、`revision_test.go`、`apply_test.go` | `viewChanges` + `noteViewChanges`（每视图最多 bump 一次）；采样/事件按真实可见变化选视图；`CompleteTask` 补归档路径失效；`task.go` 顺带 gofmt | `go test ./internal/system/` 通过；8 类场景计数 **1/1/5/4/0/5/3/2**（两个 `5` = 首次异常创建 Episode、恢复采样；§4.2b） |
| D-4 | `f4e5707` fix(agent-runtime) | `pyproject.toml`、`deploy/kubernetes/README.md`、`20-configmap.yaml`、`secret.example.yaml` | wheel `packages=["src/alertflow_agent"]`；K8s 文档与清单对齐（组件表/端口/探针/Redis/Secret 命令/`8080` 清零） | `uv build` 30,273 B / 20 模块 / `import` 成功；6 YAML `safe_load_all` 通过；残留 grep 无匹配 |
| D-5 | `91f5a51` fix(deploy) | `deploy/compose/compose.yaml`、`deploy/e2e.sh` | `db-init` 补传 `GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`；失败诊断加 `db-init` | 修后 `make e2e` 两次 exit 0（修前 `db-init` 退出 2） |
| D-6 | `ab2b8dc` fix(integration) | `RevisionCache.java` | Lua `ADVANCE`：指针前移才写指针、`next >= known` 即写值键（修「同 revision 只有第一个 query 落缓存」） | `mvn test` 20/0/0；重建 integration 镜像后接口级复验 v7 双 query 键共存、v8 推进失效 |
| D-7 | 本报告 | `docs/operations/acceptance-2026-11-17.md` | 第二轮实测与结论更正（提交哈希见 `git log`） | 占位符残留校验为 0；数字与 §1–§9 一致 |

### 11.3 第三轮（`ab2b8dc` → 收尾，按《AlertFlow 收尾整改指导》）

| # | 提交 | 文件 | 改动 | 验证 |
|---|---|---|---|---|
| E-1 | `460c6ab` fix(deploy) | `deploy/compose/compose.yaml`、`deploy/compose/postgres/init-roles.sh` | postgres healthcheck 改 `pg_isready -h 127.0.0.1 -U alertflow`（强制 TCP，避免 initdb 临时实例误报 healthy）；`init-roles.sh` 首次 `psql` 前加 30×1 s 有界重试 | 修复前：`db-init` `Connection refused` 退出 2；修复后：第三轮两次 `make e2e` 均 exit 0（61 s / 56 s）且均默认清理 |
| E-2 | 本报告 | `docs/operations/acceptance-2026-11-17.md` | 第三轮实测补记、E2E 口径区分（`make e2e` vs `bash deploy/e2e.sh`、默认清理 vs `KEEP_STACK`）、就绪竞态与修复、分支同步结果 | 占位符残留 0；数字与 §0/§3.1/§6.2/§8/§10 一致 |
| E-3 | 分支整理 | 远端 `backend`/`integration`/`agent`/`frontend`/`acceptance/2026-11-17` | 确认无独有提交后 fast-forward 到 `main`（非强推） | `git ls-remote` 五个分支与 `main` 同点；`git status` 干净 |
| E-4 | `5311548` docs | `README.md`、`docs/operations/acceptance-2026-11-17.md` | README 更正 Kubernetes 工作负载观察默认关闭且为单 namespace 只读观察，区分截图的真实模型演示与 ScriptedModel 自动 E2E 验收，统一 `make e2e` 入口并说明独立 Compose project、默认只清理本次 project、`ALERTFLOW_E2E_KEEP_STACK=1` 保留现场；验收报告更正版本边界、第三轮全量测试口径、Revision 八场景计数和三轮整改记录。该提交只涉及文档，不是业务逻辑或部署代码变更 | README 与当前 Makefile、`deploy/e2e.sh`、Compose 配置静态一致；版本节点与报告正文一致 |
