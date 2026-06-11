.PHONY: build test web web-install license-check docker-build docker-up docker-down helm-lint compose-smoke run dev

APP_NAME ?= uvoo-sqvizerver
GOCACHE ?= /tmp/go-build-cache

web:
	cd web && npm ci && npm run build

web-install:
	cd web && npm install

build:
	bash scripts/build.sh

test:
	GOCACHE=$(GOCACHE) go test ./...

license-check:
	bash scripts/license-check.sh

run: build
	./bin/$(APP_NAME)

dev:
	GOCACHE=$(GOCACHE) SQVIZ_AUTH_DEV_MODE=true SQVIZ_CLICKHOUSE_URL=http://localhost:8123 go run ./cmd/uvoo-sqvizerver

docker-build:
	docker compose build uvoo-sqviz

docker-up:
	docker compose up -d --build --remove-orphans

docker-down:
	docker compose down

helm-lint:
	helm lint charts/uvoo-sqviz

compose-smoke:
	bash scripts/smoke-compose.sh
