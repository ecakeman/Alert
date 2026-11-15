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

compose-config:
	docker compose --env-file deploy/compose/.env.example -f deploy/compose/compose.yaml config

compose-up:
	docker compose -f deploy/compose/compose.yaml up -d --build

compose-down:
	docker compose -f deploy/compose/compose.yaml down

e2e:
	bash deploy/e2e.sh
