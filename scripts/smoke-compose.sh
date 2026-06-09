#!/bin/sh
set -eu

COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-uvoo_dbviz_smoke}"
export COMPOSE_PROJECT_NAME
export DBVIZ_BIND_ADDR="${DBVIZ_BIND_ADDR:-127.0.0.1}"
export DBVIZ_PUBLIC_HOST="${DBVIZ_PUBLIC_HOST:-127.0.0.1}"
export DBVIZ_HTTP_PORT="${DBVIZ_HTTP_PORT:-18080}"
export DBVIZ_CLICKHOUSE_HTTP_PORT="${DBVIZ_CLICKHOUSE_HTTP_PORT:-18123}"
export DBVIZ_CLICKHOUSE_NATIVE_PORT="${DBVIZ_CLICKHOUSE_NATIVE_PORT:-19000}"
export DBVIZ_POSTGRES_PORT="${DBVIZ_POSTGRES_PORT:-15432}"
export DBVIZ_POSTGREST_PORT="${DBVIZ_POSTGREST_PORT:-13000}"
export DBVIZ_KEYCLOAK_PORT="${DBVIZ_KEYCLOAK_PORT:-18089}"
export DBVIZ_OTEL_GRPC_PORT="${DBVIZ_OTEL_GRPC_PORT:-14317}"
export DBVIZ_OTEL_HTTP_PORT="${DBVIZ_OTEL_HTTP_PORT:-14318}"
if [ -n "${DBVIZ_SMOKE_BASE_URL:-}" ]; then
    BASE_URL="$DBVIZ_SMOKE_BASE_URL"
else
    BASE_URL="http://${DBVIZ_BIND_ADDR}:${DBVIZ_HTTP_PORT}"
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
        docker compose logs --no-color uvoo-dbviz >&2 || true
        echo "uvoo-dbviz did not become healthy" >&2
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
