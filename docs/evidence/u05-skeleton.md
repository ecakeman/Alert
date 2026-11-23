# U05 Go 骨架与前端移植验收（T200 系列）

- 日期：2026-11-23
- 工程根：`/home/sancho/workspace/projects/Alert`
- 模块：`github.com/ecakeman/alert`
- Go：1.27.4（`~/.local/go`）｜Gin：v1.12.0（直接依赖）
- 原则：展示级骨架，只做必要最小集；全量/E2E/真实外部依赖默认不跑。

## 1. 目标与范围

只搭骨架：模块根、六层目录、domain 迁移、Gin 分层、bootstrap/cmd 启动、前端移植、证据随迁。不含业务实现与真实外部依赖。

## 2. 环境

- Go 1.27.4；Node v22.23.1；npm 10.9.8
- GOPROXY=`goproxy.cn`；npm registry=`npmmirror`

## 3. 目录与分层

- `cmd/system-core`
- `internal/{bootstrap,config,application,domain,infrastructure,transport/http}`
- `transport/http/{dto,response,httperr,middleware,handler,router}`
- `migrations` / `contracts` / `deploy` 仅 README 占位
- 依赖方向：`cmd→bootstrap→transport→application→domain`；`infrastructure` 仅占位

## 4. 依赖

- 直接依赖仅 `github.com/gin-gonic/gin v1.12.0`（`go mod tidy` 后非 indirect）
- 无 pgx / kafka / grpc / k8s

## 5. domain 迁移

- 9 源 + 7 测试 = 16 文件；`go test ./internal/domain/...`：**14 用例 PASS**
- `go list -deps` 依赖 **CLEAN**；无 `alertflow` 残留引用

## 6. Gin 路由

- `/healthz` → `200 {"status":"ok","checks":[]}`
- `/readyz` → `200 {"status":"ready","checks":[]}`
- `/api/v1` 18 条 501 壳：dashboard/summary；services(CRUD)；targets(list/create/patch/delete)；episodes(list/get)；service timeline/events；episode timeline/artifacts；retry-diagnosis/retry-archive
- 中间件链：Recovery → RequestID → AccessLog → Timeout（CORS 默认不挂）
- 错误体：`{"error":{"code","message","request_id"}}`

## 7. 前端移植

- `apps/console` 复制（26 个 src 文件，352K）
- 改动 3 文件：`src/api/client.ts`、`vite.config.ts`、新增 `.env.example`
- `npm install`（110 包）+ `npm run build` ✓，生成 `dist`

## 8. 验收 A1~A11

| 编号 | 判据 | 结果 |
|---|---|---|
| A1 | 单一模块根、`go list -m` | PASS |
| A2 | 公开路由仅 `router.go` 注册 | PASS |
| A3 | 直接依赖 ≤3（仅 gin） | PASS |
| A4 | domain 14 用例 | PASS |
| A5 | 探针 + 零依赖启动 | PASS |
| A6 | 501 + NOT_IMPLEMENTED + request_id | PASS |
| A7 | panic→500（Recovery 单测） | PASS |
| A8 | `go build` / `go vet` | PASS |
| A9 | 前端 `npm run build` | PASS |
| A10 | 六层目录形状 | PASS |
| A11 | 6 份证据随迁 | PASS |

## 9. 限制与风险 / 未执行

- 18 条业务路由仅 501 壳，未实现业务逻辑。
- 未接入 PostgreSQL/Kafka/Redis/gRPC/模型/MCP/A2A/RAG。
- 容器化（Docker/Compose/K8s）未验收。
- 全量测试 / E2E / 真实外部依赖未执行（按测试总原则）。
- 前端 `npm run dev` 联调未做（preview 无代理属已知限制）。