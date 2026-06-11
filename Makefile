.PHONY: build test web web-install license-check docker-build docker-up docker-down helm-lint helm-package package clean-dist dist release compose-smoke run dev

APP_NAME ?= uvoo-sqvizerver
GOCACHE ?= /tmp/go-build-cache
CHART_DIR ?= charts/uvoo-dbviz

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
	helm lint $(CHART_DIR)

helm-package:
	mkdir -p dist
	helm package $(CHART_DIR) --destination dist $(if $(VERSION),--version $(patsubst v%,%,$(VERSION)) --app-version $(VERSION),)
	bash scripts/checksums.sh

package:
	bash scripts/package.sh

clean-dist:
	rm -rf dist

dist: clean-dist package helm-package

release:
	bash scripts/release.sh

compose-smoke:
	bash scripts/smoke-compose.sh
