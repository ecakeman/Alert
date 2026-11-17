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

# 运行时要用的 env 文件；默认 deploy/compose/.env（由 .env.example 复制并填写口令）。
ENV_FILE ?= deploy/compose/.env

compose-config:
	docker compose --env-file deploy/compose/.env.example -f deploy/compose/compose.yaml config

compose-up:
	docker compose --env-file $(ENV_FILE) -f deploy/compose/compose.yaml up -d --build

compose-down:
	docker compose --env-file $(ENV_FILE) -f deploy/compose/compose.yaml down

e2e:
	docker compose --env-file $(ENV_FILE) -f deploy/compose/compose.yaml up -d --build
	bash deploy/e2e.sh
