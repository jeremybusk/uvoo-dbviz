package clickhouse

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"uvoo-dbviz/internal/config"
)

func TestBuildTimeseriesSQLScopesTenantAndAllowlistsColumns(t *testing.T) {
	ds := config.Dataset{
		ID:           "logs",
		Table:        "otel_logs",
		TimeColumn:   "timestamp",
		TenantColumn: "tenant_id",
		Dimensions:   []string{"service_name"},
		Filters:      []string{"severity"},
		FilterOperators: map[string][]string{
			"severity": {"eq"},
		},
		Measures:           []string{"_rows"},
		Aggregations:       []string{"count"},
		DefaultMeasure:     "_rows",
		DefaultAggregation: "count",
	}
	sql, err := BuildTimeseriesSQL(QueryRequest{
		Dataset:       "logs",
		From:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:            time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
		GroupBy:       "service_name",
		Filters:       map[string]string{"severity": "error"},
		BucketSeconds: 60,
		Limit:         500,
	}, ds, "tenant-a", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"`tenant_id` = 'tenant-a'", "`severity` = 'error'", "GROUP BY ts, series", "LIMIT 500"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("sql missing %q:\n%s", want, sql)
		}
	}
}

func TestBuildTimeseriesSQLRejectsUnexpectedFilter(t *testing.T) {
	ds := config.Dataset{
		ID:                 "logs",
		Table:              "otel_logs",
		TimeColumn:         "timestamp",
		TenantColumn:       "tenant_id",
		Measures:           []string{"_rows"},
		Aggregations:       []string{"count"},
		DefaultMeasure:     "_rows",
		DefaultAggregation: "count",
	}
	_, err := BuildTimeseriesSQL(QueryRequest{Dataset: "logs", Filters: map[string]string{"1=1": "x"}}, ds, "tenant-a", 100)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestBuildTimeseriesSQLRejectsUnexpectedAggregation(t *testing.T) {
	ds := config.Dataset{
		ID:                 "metrics",
		Table:              "otel_metrics",
		TimeColumn:         "timestamp",
		TenantColumn:       "tenant_id",
		Measures:           []string{"value"},
		Aggregations:       []string{"avg"},
		DefaultMeasure:     "value",
		DefaultAggregation: "avg",
	}
	_, err := BuildTimeseriesSQL(QueryRequest{Dataset: "metrics", Measure: "value", Aggregation: "sum"}, ds, "tenant-a", 100)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewClientDoesNotMutateProvidedHTTPClient(t *testing.T) {
	shared := &http.Client{}
	client := NewClient(config.ClickHouseConfig{Timeout: 5 * time.Second}, shared)
	if shared.Timeout != 0 {
		t.Fatalf("shared timeout = %s, want zero", shared.Timeout)
	}
	if client.http == shared {
		t.Fatal("expected NewClient to copy the provided HTTP client")
	}
	if client.http.Timeout != 5*time.Second {
		t.Fatalf("client timeout = %s, want 5s", client.http.Timeout)
	}
}
