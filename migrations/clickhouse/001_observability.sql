CREATE TABLE IF NOT EXISTS otel_logs (
    tenant_id String,
    timestamp DateTime64(3, 'UTC'),
    service_name LowCardinality(String),
    severity LowCardinality(String),
    host_name LowCardinality(String),
    trace_id String,
    body String,
    attributes JSON
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, service_name, timestamp)
TTL toDateTime(timestamp) + INTERVAL 30 DAY;

CREATE TABLE IF NOT EXISTS otel_traces (
    tenant_id String,
    timestamp DateTime64(3, 'UTC'),
    service_name LowCardinality(String),
    span_name LowCardinality(String),
    status_code LowCardinality(String),
    trace_id String,
    span_id String,
    duration_ms Float64,
    attributes JSON
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, service_name, timestamp, trace_id)
TTL toDateTime(timestamp) + INTERVAL 14 DAY;

CREATE TABLE IF NOT EXISTS otel_metrics (
    tenant_id String,
    timestamp DateTime64(3, 'UTC'),
    service_name LowCardinality(String),
    metric_name LowCardinality(String),
    value Float64,
    attributes JSON
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, metric_name, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY;

