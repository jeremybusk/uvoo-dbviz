#!/bin/sh
set -eu

OTEL_ENDPOINT="${OTEL_ENDPOINT:-http://otel-collector:4318}"
CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-clickhouse}"
CLICKHOUSE_PORT="${CLICKHOUSE_PORT:-9000}"
CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-clickhouse}"
TENANT_ID="${TENANT_ID:-dev}"

wget -qO- \
  --header='Content-Type: application/json' \
  --post-data="{
    \"resourceLogs\": [{
      \"resource\": {\"attributes\": [
        {\"key\":\"service.name\",\"value\":{\"stringValue\":\"otel-sample\"}},
        {\"key\":\"tenant.id\",\"value\":{\"stringValue\":\"$TENANT_ID\"}}
      ]},
      \"scopeLogs\": [{\"logRecords\": [{
        \"timeUnixNano\":\"$(date +%s)000000000\",
        \"severityText\":\"INFO\",
        \"body\":{\"stringValue\":\"sample otlp log from local emitter\"},
        \"attributes\":[{\"key\":\"host.name\",\"value\":{\"stringValue\":\"otel-sample\"}}]
      }]}]
    }]
  }" \
  "$OTEL_ENDPOINT/v1/logs" >/dev/null

wget -qO- \
  --header='Content-Type: application/json' \
  --post-data="{
    \"resourceSpans\": [{
      \"resource\": {\"attributes\": [
        {\"key\":\"service.name\",\"value\":{\"stringValue\":\"otel-sample\"}},
        {\"key\":\"tenant.id\",\"value\":{\"stringValue\":\"$TENANT_ID\"}}
      ]},
      \"scopeSpans\": [{\"spans\": [{
        \"traceId\":\"11111111111111111111111111111111\",
        \"spanId\":\"2222222222222222\",
        \"name\":\"GET /sample\",
        \"kind\":2,
        \"startTimeUnixNano\":\"$(date +%s)000000000\",
        \"endTimeUnixNano\":\"$(date +%s)500000000\",
        \"status\":{\"code\":1}
      }]}]
    }]
  }" \
  "$OTEL_ENDPOINT/v1/traces" >/dev/null

if [ -x /create-otel-normalizers.sh ] || [ -f /create-otel-normalizers.sh ]; then
  CLICKHOUSE_HOST="$CLICKHOUSE_HOST" \
  CLICKHOUSE_PORT="$CLICKHOUSE_PORT" \
  CLICKHOUSE_PASSWORD="$CLICKHOUSE_PASSWORD" \
  DBVIZ_OTEL_DEFAULT_TENANT="$TENANT_ID" \
  DBVIZ_OTEL_NORMALIZER_WAIT_SECONDS="${DBVIZ_OTEL_NORMALIZER_WAIT_SECONDS:-15}" \
    sh /create-otel-normalizers.sh || true
fi

clickhouse-client --host "$CLICKHOUSE_HOST" --port "$CLICKHOUSE_PORT" --password "$CLICKHOUSE_PASSWORD" --multiquery <<SQL
INSERT INTO otel_logs (tenant_id, timestamp, service_name, severity, host_name, trace_id, body, attributes) VALUES
('$TENANT_ID', now64(3), 'otel-sample', 'info', 'otel-sample', '11111111111111111111111111111111', 'sample otlp log from local emitter', '{}');

INSERT INTO otel_traces (tenant_id, timestamp, service_name, span_name, status_code, trace_id, span_id, duration_ms, attributes) VALUES
('$TENANT_ID', now64(3), 'otel-sample', 'GET /sample', 'ok', '11111111111111111111111111111111', '2222222222222222', 500, '{}');

INSERT INTO otel_metrics (tenant_id, timestamp, service_name, metric_name, value, attributes) VALUES
('$TENANT_ID', now64(3), 'otel-sample', 'sample.requests', 1, '{}');
SQL

echo "emitted OTLP sample telemetry and inserted normalized ClickHouse rows"
