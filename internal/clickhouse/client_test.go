package clickhouse

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"uvoo-sqviz/internal/config"
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

func TestBuildEventsSQLScopesTenantSearchesAndLimits(t *testing.T) {
	ds := config.Dataset{
		ID:            "logs",
		Table:         "otel_logs",
		TimeColumn:    "timestamp",
		TenantColumn:  "tenant_id",
		Filters:       []string{"severity"},
		EventColumns:  []string{"timestamp", "service_name", "severity", "body"},
		SearchColumns: []string{"body", "service_name"},
	}
	sql, err := BuildEventsSQL(QueryRequest{
		Dataset: "logs",
		From:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
		Search:  "timeout",
		Filters: map[string]string{"severity": "error"},
		Limit:   100,
	}, ds, "tenant-a", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`tenant_id` = 'tenant-a'",
		"`severity` = 'error'",
		"positionCaseInsensitive(toString(`body`), 'timeout') > 0",
		"ORDER BY `timestamp` DESC",
		"LIMIT 100",
		"`body` AS `body`",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("sql missing %q:\n%s", want, sql)
		}
	}
}

func TestBuildEventsSQLRejectsInvalidEventColumn(t *testing.T) {
	ds := config.Dataset{
		ID:           "logs",
		Table:        "otel_logs",
		TimeColumn:   "timestamp",
		TenantColumn: "tenant_id",
		EventColumns: []string{"body; DROP TABLE logs"},
	}
	_, err := BuildEventsSQL(QueryRequest{Dataset: "logs"}, ds, "tenant-a", 100)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestBuildCustomSQLRequiresParameterizedTenantAndTime(t *testing.T) {
	ds := config.Dataset{
		ID:               "logs",
		Table:            "otel_logs",
		TimeColumn:       "timestamp",
		TenantColumn:     "tenant_id",
		MaxLookbackHours: 24,
	}
	built, err := BuildCustomSQL(QueryRequest{
		Dataset: "logs",
		From:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
		SQL:     "SELECT service_name, count() AS value FROM otel_logs WHERE tenant_id = {tenant:String} AND timestamp >= {from:DateTime} AND timestamp < {to:DateTime} GROUP BY service_name",
		Limit:   100,
	}, ds, "tenant-a", 1000, CustomSQLExplore)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SELECT *", "LIMIT {limit:UInt64}", "FORMAT JSONEachRow"} {
		if !strings.Contains(built.SQL, want) {
			t.Fatalf("sql missing %q:\n%s", want, built.SQL)
		}
	}
	if built.Params["tenant"] != "tenant-a" || built.Params["limit"] != "100" {
		t.Fatalf("unexpected params: %#v", built.Params)
	}
}

func TestBuildCustomSQLRejectsUnsafeStatements(t *testing.T) {
	ds := config.Dataset{ID: "logs", Table: "otel_logs", TimeColumn: "timestamp", TenantColumn: "tenant_id"}
	_, err := BuildCustomSQL(QueryRequest{
		Dataset: "logs",
		SQL:     "SELECT * FROM otel_logs; DROP TABLE otel_logs",
	}, ds, "tenant-a", 100, CustomSQLExplore)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestBuildCustomSQLAlertAllowsNonNumericResultColumns(t *testing.T) {
	ds := config.Dataset{ID: "logs", Table: "otel_logs", TimeColumn: "timestamp", TenantColumn: "tenant_id"}
	_, err := BuildCustomSQL(QueryRequest{
		Dataset: "logs",
		SQL:     "SELECT count() AS total FROM otel_logs WHERE tenant_id = {tenant:String} AND timestamp >= {from:DateTime} AND timestamp < {to:DateTime}",
	}, ds, "tenant-a", 100, CustomSQLAlert)
	if err != nil {
		t.Fatal(err)
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
