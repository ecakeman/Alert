.PHONY: build test proto compose-config compose-up compose-down e2e

build:
	cd services/system-core && go build ./...
	cd services/integration && mvn -B -s maven-settings.xml -DskipTests package
	cd apps/console && npm ci && npm run build

test:
	cd services/system-core && go test ./...
	cd services/integration && mvn -B -s maven-settings.xml test
	cd services/agent-runtime && uv run pytest -q

proto:
	@echo "proto sources live in contracts/proto; generate from each service"

# 运行时 env 文件：口令与端口等（默认 deploy/compose/.env，由 .env.example 复制并填写）。
ENV_FILE ?= deploy/compose/.env
# 镜像变量模板：非机密的基础镜像/运行镜像默认值，与 deploy/e2e.sh 共用同一份来源。
IMAGES_ENV_FILE ?= deploy/images.env.example

# 校验 compose 配置：默认用 deploy/compose/.env；CI 等场景可 ENV_FILE=deploy/compose/.env.example 覆盖。
compose-config:
	docker compose --env-file $(IMAGES_ENV_FILE) --env-file $(ENV_FILE) -f deploy/compose/compose.yaml config

compose-up:
	docker compose --env-file $(IMAGES_ENV_FILE) --env-file $(ENV_FILE) -f deploy/compose/compose.yaml up --build -d

compose-down:
	docker compose --env-file $(IMAGES_ENV_FILE) --env-file $(ENV_FILE) -f deploy/compose/compose.yaml down

# 单一入口：E2E 编排（启动、演练、失败诊断、退出清理）全部收敛到 deploy/e2e.sh。
e2e:
	bash deploy/e2e.sh
