#!/bin/sh
set -eu

COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-uvoo_sqviz_smoke}"
export COMPOSE_PROJECT_NAME
export SQVIZ_BIND_ADDR="${SQVIZ_BIND_ADDR:-127.0.0.1}"
export SQVIZ_PUBLIC_HOST="${SQVIZ_PUBLIC_HOST:-127.0.0.1}"
export SQVIZ_HTTP_PORT="${SQVIZ_HTTP_PORT:-18080}"
export SQVIZ_CLICKHOUSE_HTTP_PORT="${SQVIZ_CLICKHOUSE_HTTP_PORT:-18123}"
export SQVIZ_CLICKHOUSE_NATIVE_PORT="${SQVIZ_CLICKHOUSE_NATIVE_PORT:-19000}"
export SQVIZ_POSTGRES_PORT="${SQVIZ_POSTGRES_PORT:-15432}"
export SQVIZ_POSTGREST_PORT="${SQVIZ_POSTGREST_PORT:-13000}"
export SQVIZ_KEYCLOAK_PORT="${SQVIZ_KEYCLOAK_PORT:-18089}"
export SQVIZ_OTEL_GRPC_PORT="${SQVIZ_OTEL_GRPC_PORT:-14317}"
export SQVIZ_OTEL_HTTP_PORT="${SQVIZ_OTEL_HTTP_PORT:-14318}"
if [ -n "${SQVIZ_SMOKE_BASE_URL:-}" ]; then
    BASE_URL="$SQVIZ_SMOKE_BASE_URL"
else
    BASE_URL="http://${SQVIZ_BIND_ADDR}:${SQVIZ_HTTP_PORT}"
fi

cleanup() {
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

retry_json() {
    filter="$1"
    shift
    deadline=$(( $(date +%s) + 90 ))
    while :; do
        if response="$("$@" 2>/dev/null)" && printf '%s' "$response" | jq -e "$filter" >/dev/null; then
            return 0
        fi
        if [ "$(date +%s)" -ge "$deadline" ]; then
            "$@" >&2 || true
            return 1
        fi
        sleep 3
    done
}

docker compose up -d --build --remove-orphans

deadline=$(( $(date +%s) + 180 ))
until curl -fsS "$BASE_URL/healthz" >/dev/null; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
        docker compose logs --no-color uvoo-sqviz >&2 || true
        echo "uvoo-sqviz did not become healthy" >&2
        exit 1
    fi
    sleep 3
done

retry_json '.datasets | length >= 3' curl -fsS "$BASE_URL/api/config"

retry_json 'length >= 1' curl -fsS \
    -H 'X-Dev-Tenant: dev' \
    -H 'X-Dev-Email: smoke@example.local' \
    -H 'Content-Type: application/json' \
    -d '{}' \
    "$BASE_URL/api/session/sync"

retry_json '.rows | length >= 1' curl -fsS \
    -H 'X-Dev-Tenant: dev' \
    -H 'X-Dev-Email: smoke@example.local' \
    -H 'Content-Type: application/json' \
    -d '{"dataset":"logs","groupBy":"service_name","measure":"_rows","aggregation":"count","limit":50}' \
    "$BASE_URL/api/query"

retry_json 'length >= 1' curl -fsS \
    -H 'X-Dev-Tenant: dev' \
    -H 'X-Dev-Email: smoke@example.local' \
    -H 'Content-Type: application/json' \
    -d '{"name":"Smoke Dashboard","layout":{"version":1,"charts":[{"title":"Smoke logs","query":{"dataset":"logs","groupBy":"service_name","measure":"_rows","aggregation":"count"},"visualization":{"type":"line"},"position":{"w":1,"h":1}}]}}' \
    "$BASE_URL/api/dashboards"

docker compose run --rm otel-sample >/dev/null

echo "compose smoke passed"
