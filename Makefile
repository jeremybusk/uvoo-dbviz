.PHONY: build test web web-install license-check docker-build docker-up docker-down helm-lint compose-smoke run dev

APP_NAME ?= uvoo-dbvizerver
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
	GOCACHE=$(GOCACHE) DBVIZ_AUTH_DEV_MODE=true DBVIZ_CLICKHOUSE_URL=http://localhost:8123 go run ./cmd/uvoo-dbvizerver

docker-build:
	docker compose build uvoo-dbviz

docker-up:
	docker compose up -d --build --remove-orphans

docker-down:
	docker compose down

helm-lint:
	helm lint charts/uvoo-dbviz

compose-smoke:
	bash scripts/smoke-compose.sh
