#!/bin/sh
set -eu

HOST="${CLICKHOUSE_HOST:-clickhouse}"
PORT="${CLICKHOUSE_PORT:-9000}"
PASSWORD="${CLICKHOUSE_PASSWORD:-clickhouse}"

clickhouse-client --host "$HOST" --port "$PORT" --password "$PASSWORD" --multiquery < /migrations/001_observability.sql

clickhouse-client --host "$HOST" --port "$PORT" --password "$PASSWORD" --multiquery <<'SQL'
INSERT INTO otel_logs (tenant_id, timestamp, service_name, severity, host_name, trace_id, body, attributes) VALUES
('dev', now64(3) - INTERVAL 55 MINUTE, 'checkout', 'info', 'app-1', 'trace-a', 'cart opened', '{}'),
('dev', now64(3) - INTERVAL 48 MINUTE, 'checkout', 'error', 'app-1', 'trace-b', 'payment retry', '{}'),
('dev', now64(3) - INTERVAL 33 MINUTE, 'api', 'info', 'app-2', 'trace-c', 'request handled', '{}'),
('dev', now64(3) - INTERVAL 14 MINUTE, 'api', 'warning', 'app-2', 'trace-d', 'slow upstream', '{}'),
('example.com', now64(3) - INTERVAL 52 MINUTE, 'worker', 'info', 'worker-1', 'trace-e', 'job complete', '{}');

INSERT INTO otel_traces (tenant_id, timestamp, service_name, span_name, status_code, trace_id, span_id, duration_ms, attributes) VALUES
('dev', now64(3) - INTERVAL 50 MINUTE, 'checkout', 'POST /cart', 'ok', 'trace-a', 'span-a', 42.1, '{}'),
('dev', now64(3) - INTERVAL 45 MINUTE, 'checkout', 'POST /pay', 'error', 'trace-b', 'span-b', 180.4, '{}'),
('dev', now64(3) - INTERVAL 30 MINUTE, 'api', 'GET /health', 'ok', 'trace-c', 'span-c', 6.5, '{}');

INSERT INTO otel_metrics (tenant_id, timestamp, service_name, metric_name, value, attributes) VALUES
('dev', now64(3) - INTERVAL 55 MINUTE, 'checkout', 'http.requests', 41, '{}'),
('dev', now64(3) - INTERVAL 45 MINUTE, 'checkout', 'http.requests', 66, '{}'),
('dev', now64(3) - INTERVAL 30 MINUTE, 'api', 'http.requests', 120, '{}'),
('dev', now64(3) - INTERVAL 15 MINUTE, 'api', 'http.requests', 155, '{}');
SQL

echo "seeded ClickHouse sample observability data"
