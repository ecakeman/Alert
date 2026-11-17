# AlertFlow 整改验收报告（方案第 12 节：12.1–12.7）

- 被测版本：`origin/main` = `d763fd1`（`1aa5b45` 之后的整改提交，首个提交 `2749ca5`）
- **双口径**（本报告同时给出两套证据，表中以「来源」列标注）：
  - **原始树** = `d763fd1` 检出的代码，未叠加任何本地修改 —— 12.1–12.6 的主体结论来自此口径；
  - **修复后树** = `d763fd1` + 本轮 **30 项未提交改动**（23 modified + 7 untracked，§11）—— §1.2、§2、§4.2、§5.3、§6、§8 的「修复后」列来自此口径。
- 基线对照：`1aa5b45`（monorepo 重排、整改提交之前）—— 用于判断 d763fd1 的失败是否由整改引入（§1.4）。
- 测试日期：2026-11-15 至 2026-11-17
- 测试环境：WSL2 Ubuntu 24.04 / Docker 29.6.1 + Compose v5.2.0 / Go 1.26.6 / JDK 25（Temurin 25.0.5+8）/ Python 3.12（uv）/ Node 22
- 口径：**只写实际执行过的结果**；未执行项单列于 §9，不计为通过；未复测项显式标注「未复测」。

> **对上一版报告的两处更正**
> 1. 删除「Go HTTP 面缺新增目标接口 `POST /api/v1/services/{id}/targets` → 405」——**误判**。接口实际存在：
>    `grep -n targets services/system-core/internal/api/server.go` → `:36` GET、`:37` POST，实测 201。
> 2. 「13 个整改提交」口径不准：`1aa5b45..d763fd1` 实为 **9 个**提交（`git log --oneline 1aa5b45..d763fd1 | wc -l` → 9），
>    `1aa5b45` 本身是 monorepo 目录重排。本报告改为「1aa5b45 基线 vs d763fd1 原始树 vs d763fd1+修复树」三栏。

---

## 0. 结论摘要

| 节 | 结果 | 来源 | 一句话 |
|---|---|---|---|
| 12.1 静态与单元测试 | 通过（修复后树） | 原始树 + 修复树 | `go test ./...` **0 FAIL**（7 个 ok 包）、Java 14/0/0、pytest 14 passed、Vue build exit 0；gofmt 仍 3 文件（均 `HEAD` 既存） |
| 12.2 契约一致性 | 部分修（有测试，无端到端） | 原始树 + 修复树 | 19 条公开路由可用；#1–#4 已改且有单测/测试库证据，#5/#6/#8 未改动，#9 原判断为误判已删 |
| 12.3 E2E 主链路 | 通过（**仅原始树**） | 原始树 | 修 4 处脚本缺陷后 `e2e.sh` exit 0；**修复版脚本未再起栈复跑** |
| 12.4 版本缓存 | 部分修（未复测） | 原始树 + 修复树 | 读缓存算法已改但**无端到端证据**；写放大**仍存** |
| 12.5 运行与故障 | P0 **已修**（单测证据） | 原始树 + 修复树 | 三处根因已改 + 10 个回归测试全绿；**无栈级复现** |
| 12.6 镜像与部署 | 部分修 | 原始树 + 修复树 | 文档/K8s/`.env.example` 已改，静态校验通过；temurin、redis 复测可拉取；**未起栈、未重建镜像、未部署** |
| 12.7 验收 | **未通过**（有阻塞项） | — | 阻塞项见 §8：写放大、Python 空 wheel 仍存；K8s 未部署；6 项栈级复跑未执行 |

阻塞项优先级一览（详见 §8）：

1. **P0**：监控调度永久停摆 —— **已修**（§5.3；确定性回归测试通过，**无栈级复现**）
2. **P1**：按 README 首条命令启动失败（空口令） —— **已改未实测**（渲染层验证通过，未 `up -d`）
3. **P1**：`RevisionCache` 读路径恒不命中 —— **已改未复测**
4. **P1**：写放大（每采样 10 条 `RESOURCE_REVISION`） —— **仍存**（`noteViews` 未改）
5. **P2**：gofmt 3 文件（`HEAD` 既存）、Python 空 wheel、无 ruff/OpenAPI 校验 —— **仍存**；`go test` 失败用例已随测试重写转绿

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

### 1.2 修复后树（`d763fd1` + 本轮未提交修复）

> 本节全部由 Lead 在**所有编辑停止后的单一代码状态**下重跑（2026-11-17 13:40–13:50），日志见 §10。
> 因此不再存在旧版所说的「全绿不对应单一代码状态」问题。

| 组件 | 命令 | 结果 |
|---|---|---|
| Go gofmt | `gofmt -l .` | **仍 3 个文件**：`cmd/system-core/artifacts.go`、`internal/gateway/server.go`、`internal/system/task.go`。但这 3 个文件在 **HEAD 版本上同样未格式化**（`git show HEAD:<path> \| gofmt -l` 三份全部命中），且 `task.go` 本轮未提交改动在 `:801`/`:863`（retry 404），与 gofmt 差异处 `task.go:368`（`RegisterFaultDocumentInput` 字段对齐）无关 → **结论：gofmt 差异是 `d763fd1` 既存问题，非本轮修复引入；本轮未修（P2 仍存）** |
| Go vet / build | `go vet ./...`；`go build ./...` | exit 0；exit 0 |
| Go 全量测试 | `go test -count=1 ./...` | **exit 0，0 FAIL**，7 个 ok：`internal/api 2.784s`、`internal/monitoring 0.193s`、`monitoring/probe/http 0.285s`、`monitoring/probe/kubernetes 0.020s`、`monitoring/scheduler 0.197s`、`monitoring/worker 0.020s`、`internal/system 5.778s`；其余 9 包 `[no test files]`。原始树的 `TestPublishPendingMarksSent` FAIL 已消失 |
| Go 重复稳定性 | `go test -count=3 ./internal/monitoring/...` | exit 0（scheduler/worker/engine 用例不 flaky） |
| Go 竞态检测 | `go test -race ...` | **无法执行**：`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`（宿主无 gcc）→ 见 §9-5 |
| Java 测试 | `JAVA_HOME=.../jdk-25.0.5+8 mvn -B -s maven-settings.xml -Dmaven.repo.local=/home/sancho/tools/m2repo -f services/integration/pom.xml test` | exit 0，**BUILD SUCCESS，Tests run: 14, Failures: 0, Errors: 0, Skipped: 0**；`BridgeAndInvocationTest 6`、**`PublicApiErrorsTest 3`**、`VirtualThreadWebTest 1`、`BoundaryAndVirtualThreadTest 4`（原始树 11 → 修复后 14） |
| Python 测试 | `uv sync --frozen && uv run pytest -q` | exit 0（Checked 59 packages）；**14 passed in 4.65s** |
| Vue 构建 | `npx vue-tsc --noEmit && npm run build` | exit 0；exit 0，**2266 modules，built in 9.41s**，产物 `DashboardView-BaeayJ-X.js 462.99 kB (gzip 155.81)`、`index-Com4i6yR.js 1,219.00 kB (gzip 392.30)`、`index-DCn_y49j.css 382.90 kB` |
| Compose 校验 | `ENV_FILE=deploy/compose/.env.example make compose-config` | exit 0，11 个 service 全解析（agent-runtime/db-init/fault/frontend/integration/kafka/kafka-init/nacos/postgres/redis/system-core） |

### 1.3 失败用例明细（原始树）

```
--- FAIL: TestPublishPendingMarksSent (0.24s)
    apply_test.go:403: unpublished 10 <nil>
```

根因（逐行核对）：`internal/system/apply.go` 的 `ApplyMonitorSample` 在提交前两次调用 `noteViews`
（`apply.go:172`、`apply.go:194`），对 `dashboard`/`services`/`episodes`/`service:<id>`/`targets:<id>` 五个视图各写一条
`RESOURCE_REVISION` outbox，一次采样 10 条；而 `internal/system/publish.go` 的 `PublishPending` 只发 `AGENT_TASK_READY`，
`UnpublishedCount` 统计 `published_at IS NULL` 全部行 → 断言失败。
运行态完整（`PublishByType("RESOURCE_REVISION")` 在 gateway 启动），**属测试未随 `562a01c` 同步，不是产品故障**。
本轮已按新语义重写该用例（§11-4）；修复后 `go test -count=1 ./internal/system/` → **ok 5.778s**，`TestPublishPendingMarksSent` 通过（§1.2）。

gofmt 差异（原始树，均非功能问题）：

- `services/system-core/cmd/system-core/artifacts.go`、`services/system-core/internal/gateway/server.go`：import 分组顺序
- `services/system-core/internal/system/task.go:368`：`RegisterFaultDocumentInput` 字段对齐

修复后 gofmt 结果：仍为 3 个文件，且三者在 **HEAD 版本上同样未格式化**（`git show HEAD:<path> | gofmt -l` 三份全部命中）
→ **gofmt 差异属 `d763fd1` 既存问题，非本轮修复引入；本轮未修（P2 仍存）**。

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
- 修复后：**15 个 `_test.go` / 51 个用例**（**+14**，与 §11 新增测试数一致）；`scheduler`、`worker` 从「无测试文件」变为 `ok`；`monitoring` 与 `internal/system` 显著加强。
- 矩阵点名场景中：队列满、同目标不重叠、配置更新生效、`Context` 取消 4 项有测试；**Worker 退出、调度竞争、结果提交失败原始树无测试**（修复后均已补，见下）。
- 实际漏网缺陷：**新建目标不采样**（§5.3），现已由 `TestEngineNewTargetGetsFirstSample` 覆盖。
- 本轮补测（**14 个用例，全部通过**）：
  - `monitoring/scheduler`（新文件 5 个）：`TestSchedulerConcurrentSyncPopDeferKeepsHeapOrder`、`TestSchedulerSyncRaceKeepsInvariants`、`TestDeferRejectsStaleVersion`、`TestDeferAppliesCurrentVersion`、`TestRemoveThenPushResetsIndex` —— 覆盖「Sync/Defer/PopDue 并发时堆序与 `inHeap`/`index` 不变式」「过期版本 Defer 不得回堆」；包内断言 `requireHeapOrder`、`countInHeap`。`go test -count=1 ./internal/monitoring/scheduler/` → **ok 0.197s**，`-count=3` 亦 ok。
  - `monitoring/worker`（新文件 3 个）：`TestWorkerSurvivesPanic`、`TestWorkerNeverDiesAcrossManyPanics`、`TestRunWithoutOptionsStillRecovers` —— panic 不终止 worker，后续 job 仍被处理。**ok 0.020s**。
  - `monitoring`（引擎 2 个）：`TestEngineNewTargetGetsFirstSample`（新建目标在一个 interval 内出首个采样，覆盖漏网缺陷 §5.3）、`TestEngineSurvivesProbePanic`。**ok 0.193s**。
  - `internal/system`（新文件 4 个）：`TestMapDBErrUniqueViolationTo409`、`TestMapDBErrPassesThroughOthers`、`TestCreateServiceDuplicateKeyIsConflict`（0.31s）、`TestRetryUnknownEpisodeIsNotFound`（0.50s）。后两条打**真实测试库**（`127.0.0.1:55433`），分别验证重复 `service_key` → 409 `ALREADY_EXISTS`、未知 episode 的 `retry-diagnosis`/`retry-archive` → 404 `EPISODE_NOT_FOUND`。**ok 5.778s**（整个包）。
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
| 1 | `PUT /api/v1/services/{id}` | 405 + 统一 `{error_code,message}` | 405 但为 Spring 默认体 `{"timestamp","status","error","path"}` | **405 + 统一体**（单测覆盖，**未端到端复测**） | `PublicApiErrors.java`（§11-5）；`PublicApiErrorsTest.methodNotAllowedUsesUnifiedErrorBody` 通过（Java 14/0/0） |
| 2 | `GET /healthz` | 无此路由 | 404，Spring 默认体 | **404 + 统一体**（单测覆盖，**未端到端复测**） | 同处理器的 404 分支；`PublicApiErrorsTest.unknownRouteUsesUnifiedErrorBody` 通过 |
| 3 | 重复 `service_key` | 409 | **500 INTERNAL**（`uniq_services_service_key_active` SQLSTATE `23505`） | **409 `ALREADY_EXISTS`**（真实测试库验证，**未端到端复测**） | `error.go mapDBErr` + `store.go`（§11-1/2）；`TestCreateServiceDuplicateKeyIsConflict` 通过（0.31s）、`TestMapDBErrUniqueViolationTo409` 通过 |
| 4 | `retry-*` 对不存在 episode | 404 | **500 INTERNAL**（`sql: no rows in result set`） | **404 `EPISODE_NOT_FOUND`**（真实测试库验证，**未端到端复测**） | `task.go`（§11-3）；`TestRetryUnknownEpisodeIsNotFound` 通过（0.50s） |
| 5 | `>1.4 MiB` 请求体 | 413/400 且信息准确 | **400** `service_key, name and authority_producer_id are required` | **未复测** | 本轮未改动；保留原结论 |
| 6 | `artifacts` 不带 `type` | 400 | **200**，静默返回 INVESTIGATION；`type=bogus` 同样 200 | **未复测** | 本轮未改动；保留原结论 |
| 7 | `GET /api/v1/targets/{id}` | 契约只声明 PATCH/DELETE | 405 | **未变**（契约与实现一致，非缺陷） | — |
| 8 | 响应头 `X-Request-ID` | 透传 Go 的 request_id | **无** | **未复测** | 本轮未改动 |
| 9 | `POST /api/v1/services/{id}/targets` | 201 新增目标 | ~~405~~ | **接口存在，实测 201** | `internal/api/server.go:37`；**上一版报告此处为误判，已删除** |

> 说明：原 §2.1 的 19 条路由矩阵与 §2.2 的 #1–#4 结论均为**修复前的原始树**实测。本轮对 #1–#4 只做到**单测/存储层验证**，未在起栈后逐一重放 HTTP 请求，
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
| 轮询 `${API_URL}/healthz`（Java 公开端口） | 60 次重试全失败 | 改为 `/readyz` | **已改（工作区，未提交）**：`:118`、`:123` |
| `docker compose exec -T agent ...` | `service "agent" is not running` | 改为 `agent-runtime` | **已改**：`:126`、`:136`、`:355` |
| `FAULT_PORT` 只从 `deploy/compose/.env` 读，不传给 compose | `Bind for 127.0.0.1:18081 failed` | 运行时同时导出到 shell 环境 | 运行期操作，未改脚本 |
| 未注入口令变量 | `password authentication failed for user "alertflow"` | 启动前 source 环境文件 | 运行期操作，未改脚本 |

校验：`bash -n deploy/e2e.sh` → exit 0；`grep -n 'healthz\|exec -T agent' deploy/e2e.sh` → 无残留（仅匹配到 `agent-runtime`）。
脚本修复后的**实际复跑结果**：**本轮未复跑**。`e2e.sh` 需要完整 11 service compose 栈（含 Kafka/Nacos），本机 6.7 GB 内存在与回归测试串行时存在 OOM 风险；故只做了 `bash -n` 与残留 grep 校验（见上）。
> 因此 §12.3 的「通过」**仅对原始树那一轮成立**；修复后的脚本未再起栈验证，**不记作通过**（§9-1）。

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

**(a) 读缓存恒不命中 —— 已改，待实测确认**

原始树：`RevisionCache.around()` 命中要求进程内 `revisions` 已知且等于指针；该 map 只在 `apply()`（Kafka 消费）更新，
请求路径从不回填；未命中时用 `revisions.getOrDefault(key, 0L)` 写指针 → 冷启动等价「直连 Go + 一次 Redis 往返 + 一次 Lua」，
且把指针写成 0 而非 Go 真实 revision。进程外探针连续两次 `around()` 观测 `loader calls=2`（期望第二次命中）。

本轮改法（`services/integration/src/main/java/com/alertflow/integration/cache/RevisionCache.java`，§11-7）：

1. 命中判据改为「**Redis 指针已知即命中**」——新增 `pointers` 作为指针的本地副本，`apply()` 时写指针；
   `around()` 判据变为 `cachedPointer(key) == pinned`；
2. **`pins`**：把同一 key 的并发未命中合并成一次回源（单飞），避免热点 key 重复打 Go；
3. 写回改用 **`refreshPointer()`** 读 Redis 真实指针，而不是 `revisions.getOrDefault(key, 0L)`（后者会把指针写成 0）。

修复后实测：**未做端到端复测**（本轮未起栈，也未用进程外探针重测 `around()` 命中率或观测 `loader calls` 是否由 2 降为 1）。
→ 本条按**「已改未复测」**记录，**不计为通过**：§8-B3 保持「已改未复测」，§9-2 记为未执行。改动仅做代码级核对（§11-7），无行为证据。

**(b) 写放大 —— 仍存**

每次采样两次 `noteViews`（`services/system-core/internal/system/apply.go:172` 与 `:194`），各 bump 5 个视图 → 一次采样 10 条 `RESOURCE_REVISION`。
原始树运行 9 分钟：`system.resource_revisions` dashboard/services/episodes 各 572、`service/targets` 559，
`RESOURCE_REVISION` outbox **2865 条**（已发布 616），采样点仅 **557** 条 → 约为必要值的 **5 倍**。
本轮 `noteViews` 未改动（`grep -rn noteViews services/system-core/internal/system/` 仍有 `apply.go:172/:194` 等 12 处调用点），
故本条在 §8 中标注**仍存**。

### 4.3 未执行

singleflight 并发击穿**压测**（本轮加的是「合并回源」实现，但**未压测**其效果）；多副本一致性（固定单副本）。

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

**修复（工作区未提交，`services/system-core/internal/monitoring/`）**：

1. `scheduler/scheduler.go`：`Item` 增加 `Version`（配置变更 / 代次变更 / 出堆时递增）；`Defer(id, due, version)` 携带出堆时的快照版本，**版本不匹配即丢弃**（过期回推不再污染堆）；`Sync` 仅在「配置变化且不在堆中」时以 `now` 重新入堆，配置未变时对在途目标 `continue`（不再抢先重推）；新增 `syncLocked/pushLocked/removeLocked/popDueLocked/deferLocked`，把 `inHeap`/`index` 的维护收敛到这几处 —— `minHeap.Push` 置 `inHeap=true`、`Pop` 置 `inHeap=false`，`deferLocked` 先 `removeLocked` 再 `pushLocked`，**彻底消除在失效 `index` 上调用 `heap.Fix` 的路径**（根因 1）。
2. `worker/worker.go`：新增 `Options{OnPanic func(queue.Job, any)}`、`RunWithOptions(...)`、`safeHandle`（在单个 job 边界 `defer recover()`，恢复后继续消费；`Run` 退化为薄包装，保持旧签名）—— 根因 2。
3. `engine.go`：`jobs.Done(...)` 与 `sched.Defer(job.Target.TargetID, due, job.Version)` 收敛到同一处统一出口的 `defer`，`ctx.Err() != nil` 时不回堆；`OnPanic` 记录 `monitor probe panicked`；tick 循环改为 `TryEnqueue(queue.Job{..., Version: item.Version})` 传递版本 —— 根因 3。
4. `queue/queue.go`：仅新增 `Job.Version uint64` 快照字段，不改队列语义。

**修复后回归测试**：`go test -count=1 ./internal/monitoring/...` → **ok**（scheduler 0.197s / worker 0.020s / monitoring 0.193s / probe-http 0.285s / probe-kubernetes 0.020s）；`go test -count=3 ./internal/monitoring/...` → exit 0（不 flaky）。用例清单见 §1.5：`TestEngineNewTargetGetsFirstSample` 覆盖「新建目标不采样」，`TestWorkerSurvivesPanic`/`TestWorkerNeverDiesAcrossManyPanics` 覆盖 worker 静默死亡，`TestSchedulerConcurrentSyncPopDeferKeepsHeapOrder`/`TestSchedulerSyncRaceKeepsInvariants`/`TestDeferRejectsStaleVersion` 覆盖根因 1 的堆序竞态与版本失效。

**局限**：`-race` 不可用（宿主无 cgo：`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`），故本条按「**无竞态检测下的确定性回归测试**」记录；且**未在起栈后复现/验证「pause 后自愈」**（§9-1）。因此 P0 记为「**已修 + 单测证据**」，**不计为端到端通过**。

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
- **四个 Dockerfile 的 `FROM` 仍是 Docker Hub 上游地址**，`deploy/images.env.example` 未被 Compose 引用 → **应用镜像国内化未落地（仍存）**。
- 本地构建产物：system-core 197 MB、integration 662 MB、agent-runtime 535 MB、frontend 77.4 MB。

### 6.2 Compose

- 干净树 `make compose-config` exit 0；`docker compose config --quiet` 通过。
- **按 README 首条命令执行失败（P1，原始树）**：`cp deploy/compose/.env.example deploy/compose/.env` 后直接起栈，
  `postgres` 报 `Error: Database is uninitialized and superuser password is not specified.` 退出，整栈失败。
  模板首节 6 个口令（`POSTGRES_PASSWORD`/`GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`/`MCP_TOKEN`/`KNOWLEDGE_MCP_TOKEN`/`A2A_TOKEN`）为空且未标注「必须填写」。
- `.env.example` 第 2 节仍是旧拓扑命名（`alertflow:18080:8080`、`agent`、`INTEGRATION_*_GRPC_TARGET=alertflow:9090 / agent:9000`）；
  README `:74`/`:103` 让人「填写 `deploy/.env`」，实际是 `deploy/compose/.env`，`.gitignore:6` 原也只忽略 `deploy/.env`。
- 修复后状态（工作区未提交）：`.env.example` 6 个口令改为 `CHANGEME_*` 占位并标「必须修改」（`POSTGRES_PASSWORD`/`GATEWAY_PASSWORD`/`KNOWLEDGE_PASSWORD`/`MAIN_MCP_TOKEN`/`KNOWLEDGE_MCP_TOKEN`/`A2A_TOKEN`，`:30/:33/:36/:39/:42/:45`）；头部用法改为带 `--env-file deploy/compose/.env`；旧拓扑名 `alertflow`→`system-core`、`agent`→`agent-runtime`（`:64/:118/:159`）。README `:73-77` 首条命令补 `--env-file deploy/compose/.env` 与「必须修改口令」提示，`:74`/`:103` 路径改为 `deploy/compose/.env`，`:88` `down` 同样补 `--env-file`。
- 验证边界：`ENV_FILE=deploy/compose/.env.example make compose-config` → **exit 0**，且渲染结果中 6 个口令均非空（直接消除 postgres `Database is uninitialized and superuser password is not specified.`）。**仅渲染层验证，未 `up -d` 起栈** → P1 记为「**已改未实测**」。

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

静态校验（本轮，工作区版）：6 个 YAML（`00`/`10`/`20`/`30`/`40` + `secret.example`）`pyyaml.safe_load_all` 全部解析成功（文档数 2/2/3/4/4/1）；残留 grep（`deploy/k8s`、`alertflow-backend`、`agent/src/alertflow_agent/http_app`、`alertflow:8080`）无匹配。**未部署、未 `kubectl apply`，K8s 侧仍只算静态校验。**

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

> 本节全部为静态文档核对；本轮 10 个文件的改动（diffstat 含 `deploy/e2e.sh` 共 +72/−58）**未逐条做渲染/运行验证**，仅做 YAML 解析、`bash -n`、`make compose-config` 与残留 grep 四类校验。

---

## 8. 阻塞项与修复状态

> 状态取值：**已修**（本轮改动 + 有实验证据）、**已改未复测**（本轮改动但无端到端复测）、**仍存**（本轮未改）、**未复测**（未改动也未再验证）。
> 不带证据的「已修」不成立：每条「已修/已改」都必须附本轮实测命令或测试名（见 §1.2、§1.5、§11 的验证列），否则一律记为「未复测」或「仍存」。

### A. P0 —— 监控调度停摆

- 状态：**已修**（工作区未提交；有确定性回归测试，**无栈级复现** → 不记为端到端通过）
- 根因与修法见 §5.3。「af-t1 停摆 / af-t2 未复现」是**原始树**事实；修复是否消除停摆只能靠合成竞态的确定性测试与栈上复跑，后者见 §9。

### B. P1 —— 交付可用性

1. **README 首条命令失败** —— **已改未实测**（§6.2：补 `--env-file` + 6 个口令占位；渲染层验证通过，未 `up -d`）。
2. **~~目标管理缺 Go 接口~~** —— **删除**。`POST /api/v1/services/{service_id}/targets` 存在（`internal/api/server.go:37`），实测 201。上一版报告为误判。
3. **`RevisionCache` 读路径恒不命中** —— 已改（§4.2a）；**端到端收益未复测**（§9-9④）。
4. **写放大** —— **仍存**（`noteViews` 未改，§4.2b）。
5. **错误映射** —— Go `23505 → 409`、`ErrNoRows → 404` 已改，并各有单测/真实测试库用例（§2.2 #3/#4）；Java 405/404 统一体已改，`PublicApiErrorsTest` 3 用例通过；`artifacts` 缺 `type` 仍为 200（**未复测/未改**）。
6. **K8s** —— **部分改、仍存未部署**（§6.3）：重复 `ALERTFLOW_GRPC_ADDR` 已删、镜像名已改、`secret.example.yaml` 路径已改、`deploy/kubernetes/README.md` 已改；探针未拆分、`secret.example` 的「Go Backend」字样保留、**未 `kubectl apply`**。

### C. P2 —— 工程质量

- `TestPublishPendingMarksSent` 已按新语义重写并通过（§11-4；`internal/system ok 5.778s`）；**gofmt 仍 3 文件（`d763fd1` 既存，未修）**；`scheduler`/`worker` 单测已补，`queue`/`probe/tcp`/`gateway`/`transport`/`db` 仍无测试文件。
- Python：`uv build` 仍为空包（1025 B）、无 `ruff` —— **仍存**（本轮未改）。
- Java：19 条公开路由仍无控制器层测试；OpenAPI 校验仍未接入 —— **仍存**。
- 全仓无 metrics；`ALERTFLOW_MONITOR_*` 未进清单 —— **仍存**。
- 构建镜像或 CI 装 gcc 以启用 `-race` —— **未做**（§9）。

---

## 9. 未执行 / 未验证（不计为通过）

1. K8s 真实部署与启动验证（无可用集群，仅静态校验）。
2. 真实模型诊断（未设置 `ALERTFLOW_LLM_API_KEY`，全部 ScriptedModel）；Knowledge Agent 未启用。
3. `make test` 全量目标（等价组件已分别验证，未整体执行）。
4. 浏览器点击级回归（仅 HTTP 可达性与历史截图）。
5. Go `-race`（宿主无 gcc，`-race requires cgo`）。
6. 压测：缓存并发击穿 / 写放大容量影响只做计数观测，未做容量压测。
7. Java 公开路由单测（19 条）与 OpenAPI 契约校验（仓内不存在）。
8. 多副本一致性（Compose 固定单副本，K8s replicas=1）。
9. **修复后需栈级复跑的项（六条，均未执行）**：① `deploy/e2e.sh` 修复版（§3.1）；② P0「pause 后自愈」（§5.3）；③ 契约 #1–#4 的 HTTP 端到端重放（§2.2）；④ `RevisionCache` 命中率 / `loader calls`（§4.2a）；⑤ 按 README 首条命令 `up -d` 起栈（§6.2）；⑥ 镜像重建（本轮只验证了「拉取可达」，未 `docker build`）。

---

## 10. 采样清单（可复现命令与产物）

- Go：`cd services/system-core && gofmt -l . && go vet ./... && go build ./... && go test -count=1 ./...`
- Java：`JAVA_HOME=/home/sancho/tools/jdk-25.0.5+8 mvn -B -s maven-settings.xml -Dmaven.repo.local=/home/sancho/tools/m2repo package`
- Python：`cd services/agent-runtime && uv sync --frozen && uv run pytest -q && uv build --out-dir /home/sancho/af-dist .`
- Vue：`cd apps/console && npm ci && npx vue-tsc --noEmit && npm run build`
- Compose：`ENV_FILE=deploy/compose/.env.example make compose-config`；`docker compose --env-file /home/sancho/.af-e2e-env -p af-t1 -f deploy/compose/compose.yaml up -d --build`
- E2E：`cd deploy && set -a && . /home/sancho/.af-e2e-env && set +a && COMPOSE_PROJECT_NAME=af-t1 FAULT_PORT=18085 bash e2e.sh`（日志 `/home/sancho/af-e2e-t7.log`）
- 监控复现脚本：`/tmp/mon-repro.sh`；P0 现场记录 `/home/sancho/af-monitor-repro.md`
- 契约矩阵：`/tmp/api_matrix.sh`（Java 公开面 19 条路由）
- 本轮验证日志：`/tmp/lead-go.log`（`gofmt -l` / `go vet` / `go build` / `go test -count=1 ./...`）、`/tmp/lead-java.log`、`/tmp/lead-py.log`、`/tmp/lead-vue.log`、`/tmp/lead-compose.log`；早期 b1 日志 `/tmp/b1-*.log`（当时工作树被并发改动，仅作参考，§1.2 的结论以 lead 日志为准）。

---

## 11. 本轮修复清单（未提交）

> 状态：全部位于工作区，**未 `git add`/`commit`/`push`**（任务约束）。「验证」列只写实际执行过的检查。

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
| 28 | `deploy/e2e.sh` | `:118/:123` `/healthz`→`/readyz`；`:126/:136/:355` `exec -T agent`→`agent-runtime` | `bash -n` exit 0；`grep` 无残留；**未复跑**（§3.1、§9-9①） |
| 29 | `docs/operations/acceptance-2026-11-17.md`（本文件，新） | 本报告 | — |

合计：**23 modified + 7 untracked = 30 项**（`git status --short`），其中新增测试文件 4 个、新增 Java 文件 2 个。

其他本轮观察到的事实：

- Vue 主包 `index-Com4i6yR.js` = **1,219.00 kB**（`gzip 392.30 kB`；element-plus 全量未拆分，**仍存**）。
- `noteViews` 未动（§4.2b，写放大**仍存**）。
- `git` 侧未做任何 `add`/`commit`/`push`；远程 `origin/main` 仍为 `d763fd1`。
