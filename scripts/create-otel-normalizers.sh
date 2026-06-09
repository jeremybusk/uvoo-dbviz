#!/bin/sh
set -eu

HOST="${CLICKHOUSE_HOST:-clickhouse}"
PORT="${CLICKHOUSE_PORT:-9000}"
USER="${CLICKHOUSE_USER:-default}"
PASSWORD="${CLICKHOUSE_PASSWORD:-clickhouse}"
DATABASE="${CLICKHOUSE_DATABASE:-default}"
TENANT_FALLBACK="${DBVIZ_OTEL_DEFAULT_TENANT:-dev}"
WAIT_SECONDS="${DBVIZ_OTEL_NORMALIZER_WAIT_SECONDS:-90}"

client() {
    clickhouse-client \
        --host "$HOST" \
        --port "$PORT" \
        --user "$USER" \
        --password "$PASSWORD" \
        --database "$DATABASE" \
        "$@"
}

table_exists() {
    name="$1"
    count="$(client --query "SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = '$name'")"
    test "$count" = "1"
}

create_view() {
    name="$1"
    source="$2"
    sql="$3"
    if ! table_exists "$source"; then
        echo "skipping $name: source table $source does not exist"
        return 0
    fi
    if client --query "$sql"; then
        echo "created or verified $name from $source"
    else
        echo "skipping $name: $source schema is not compatible with this normalizer" >&2
    fi
}

deadline=$(( $(date +%s) + WAIT_SECONDS ))
while :; do
    if table_exists otelcol_logs || table_exists otelcol_traces || table_exists otelcol_metrics_gauge || table_exists otelcol_metrics_sum; then
        break
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
        echo "collector tables were not created within ${WAIT_SECONDS}s; normalizers can be created by rerunning this script after telemetry arrives"
        exit 0
    fi
    sleep 3
done

create_view dbviz_mv_otelcol_logs otelcol_logs "
CREATE MATERIALIZED VIEW IF NOT EXISTS dbviz_mv_otelcol_logs
TO otel_logs
AS
SELECT
    multiIf(
        mapContains(ResourceAttributes, 'tenant_id'), ResourceAttributes['tenant_id'],
        mapContains(ResourceAttributes, 'tenant.id'), ResourceAttributes['tenant.id'],
        mapContains(ResourceAttributes, 'dbviz.tenant_id'), ResourceAttributes['dbviz.tenant_id'],
        '$TENANT_FALLBACK'
    ) AS tenant_id,
    toDateTime64(Timestamp, 3, 'UTC') AS timestamp,
    if(ServiceName = '', 'unknown', ServiceName) AS service_name,
    if(SeverityText = '', 'info', SeverityText) AS severity,
    multiIf(
        mapContains(ResourceAttributes, 'host.name'), ResourceAttributes['host.name'],
        mapContains(ResourceAttributes, 'host_name'), ResourceAttributes['host_name'],
        ''
    ) AS host_name,
    TraceId AS trace_id,
    Body AS body,
    toJSONString(mapConcat(ResourceAttributes, LogAttributes)) AS attributes
FROM otelcol_logs"

create_view dbviz_mv_otelcol_traces otelcol_traces "
CREATE MATERIALIZED VIEW IF NOT EXISTS dbviz_mv_otelcol_traces
TO otel_traces
AS
SELECT
    multiIf(
        mapContains(ResourceAttributes, 'tenant_id'), ResourceAttributes['tenant_id'],
        mapContains(ResourceAttributes, 'tenant.id'), ResourceAttributes['tenant.id'],
        mapContains(ResourceAttributes, 'dbviz.tenant_id'), ResourceAttributes['dbviz.tenant_id'],
        '$TENANT_FALLBACK'
    ) AS tenant_id,
    toDateTime64(Timestamp, 3, 'UTC') AS timestamp,
    if(ServiceName = '', 'unknown', ServiceName) AS service_name,
    SpanName AS span_name,
    toString(StatusCode) AS status_code,
    TraceId AS trace_id,
    SpanId AS span_id,
    toFloat64(Duration) / 1000000 AS duration_ms,
    toJSONString(mapConcat(ResourceAttributes, SpanAttributes)) AS attributes
FROM otelcol_traces"

for source in otelcol_metrics_gauge otelcol_metrics_sum; do
    create_view "dbviz_mv_${source}" "$source" "
CREATE MATERIALIZED VIEW IF NOT EXISTS dbviz_mv_${source}
TO otel_metrics
AS
SELECT
    multiIf(
        mapContains(ResourceAttributes, 'tenant_id'), ResourceAttributes['tenant_id'],
        mapContains(ResourceAttributes, 'tenant.id'), ResourceAttributes['tenant.id'],
        mapContains(ResourceAttributes, 'dbviz.tenant_id'), ResourceAttributes['dbviz.tenant_id'],
        '$TENANT_FALLBACK'
    ) AS tenant_id,
    toDateTime64(TimeUnix, 3, 'UTC') AS timestamp,
    if(ServiceName = '', 'unknown', ServiceName) AS service_name,
    MetricName AS metric_name,
    toFloat64(Value) AS value,
    toJSONString(mapConcat(ResourceAttributes, Attributes)) AS attributes
FROM $source"
done
