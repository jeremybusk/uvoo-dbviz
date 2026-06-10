package alert

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"uvoo-dbviz/internal/clickhouse"
	"uvoo-dbviz/internal/config"
)

func TestEvaluateFiringRuleRecordsAndNotifies(t *testing.T) {
	ch := fakeClickHouse(t, `{"value":12}`)
	var webhookCalls int32
	webhookURL := "http://webhook.local/alerts"
	webhookClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&webhookCalls, 1)
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.String() != webhookURL {
			t.Fatalf("webhook URL = %s", r.URL.String())
		}
		return textResponse(http.StatusAccepted, ""), nil
	})}

	worker := testWorker(ch)
	worker.http = webhookClient
	var recordedStatus string
	var deliveredStatus string
	worker.SetIncidentRecorder(func(_ context.Context, rule Rule, status string, value float64, _ map[string]any, fingerprint string, _ int) (RecordResult, error) {
		recordedStatus = status
		if rule.ID != "rule-1" || value != 12 || fingerprint != "dev:rule-1" {
			t.Fatalf("unexpected record call: rule=%+v value=%v fingerprint=%s", rule, value, fingerprint)
		}
		return RecordResult{IncidentID: "incident-1", ShouldNotify: true}, nil
	})
	worker.SetNotificationRecorder(func(_ context.Context, _ Rule, incidentID string, _ ContactEndpoint, result DeliveryResult, _ map[string]any) error {
		if incidentID != "incident-1" {
			t.Fatalf("incidentID = %s", incidentID)
		}
		deliveredStatus = result.Status
		return nil
	})

	worker.evaluate(context.Background(), testRule(webhookURL, 10))

	if recordedStatus != "firing" {
		t.Fatalf("recorded status = %q", recordedStatus)
	}
	if deliveredStatus != "success" {
		t.Fatalf("delivered status = %q", deliveredStatus)
	}
	if atomic.LoadInt32(&webhookCalls) != 1 {
		t.Fatalf("webhook calls = %d", webhookCalls)
	}
}

func TestEvaluateSuppressesNotificationWhenRecorderRequestsCooldown(t *testing.T) {
	ch := fakeClickHouse(t, `{"value":12}`)
	var webhookCalls int32
	worker := testWorker(ch)
	worker.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&webhookCalls, 1)
		return textResponse(http.StatusOK, ""), nil
	})}
	worker.SetIncidentRecorder(func(context.Context, Rule, string, float64, map[string]any, string, int) (RecordResult, error) {
		return RecordResult{IncidentID: "incident-1", Deduped: true, ShouldNotify: false}, nil
	})

	worker.evaluate(context.Background(), testRule("http://webhook.local/alerts", 10))

	if atomic.LoadInt32(&webhookCalls) != 0 {
		t.Fatalf("webhook calls = %d", webhookCalls)
	}
}

func TestEvaluateWaitsForSustainedCondition(t *testing.T) {
	ch := fakeClickHouse(t, `{"value":12}`)
	var webhookCalls int32
	worker := testWorker(ch)
	worker.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&webhookCalls, 1)
		return textResponse(http.StatusOK, ""), nil
	})}
	var records int32
	worker.SetIncidentRecorder(func(context.Context, Rule, string, float64, map[string]any, string, int) (RecordResult, error) {
		atomic.AddInt32(&records, 1)
		return RecordResult{IncidentID: "incident-1", ShouldNotify: true}, nil
	})
	rule := testRule("http://webhook.local/alerts", 10)
	rule.Condition.For = "5m"

	worker.evaluate(context.Background(), rule)

	if atomic.LoadInt32(&records) != 0 || atomic.LoadInt32(&webhookCalls) != 0 {
		t.Fatalf("condition fired before hold window: records=%d webhooks=%d", records, webhookCalls)
	}

	worker.pending[ruleFingerprint(rule)] = time.Now().Add(-6 * time.Minute)
	worker.evaluate(context.Background(), rule)

	if atomic.LoadInt32(&records) != 1 || atomic.LoadInt32(&webhookCalls) != 1 {
		t.Fatalf("condition did not fire after hold window: records=%d webhooks=%d", records, webhookCalls)
	}
}

func TestEvaluateResolvedRuleRecordsClear(t *testing.T) {
	ch := fakeClickHouse(t, `{"value":3}`)
	worker := testWorker(ch)
	var recordedStatus string
	worker.SetIncidentRecorder(func(_ context.Context, _ Rule, status string, value float64, payload map[string]any, _ string, _ int) (RecordResult, error) {
		recordedStatus = status
		if value != 3 || payload["resolvedAt"] == nil {
			t.Fatalf("unexpected resolved payload: value=%v payload=%#v", value, payload)
		}
		return RecordResult{}, nil
	})

	worker.evaluate(context.Background(), testRule("http://example.invalid/webhook", 10))

	if recordedStatus != "resolved" {
		t.Fatalf("recorded status = %q", recordedStatus)
	}
}

func TestEvaluateRowsSupportsRowCountAndTextMatch(t *testing.T) {
	rows := []map[string]any{
		{"message": "request timeout", "value": 1, "fingerprint": "svc-a"},
		{"message": "ok", "value": 1, "fingerprint": "svc-b"},
	}
	rowCount := EvaluateRows(Condition{Type: "row_count", Operator: "gte", Threshold: 2}, rows, "rule")
	if len(rowCount) != 1 || rowCount[0].Value != 2 {
		t.Fatalf("row count evaluations = %#v", rowCount)
	}
	text := EvaluateRows(Condition{Type: "text_match", Field: "message", Operator: "contains", Value: "timeout"}, rows, "rule")
	if len(text) != 1 || text[0].Fingerprint != "rule:svc-a" {
		t.Fatalf("text evaluations = %#v", text)
	}
}

func TestEvaluateAnyRowsRecordsPerRowFingerprints(t *testing.T) {
	ch := fakeClickHouse(t, `{"value":2,"message":"timeout","fingerprint":"svc-a"}
{"value":3,"message":"panic","fingerprint":"svc-b"}`)
	worker := testWorker(ch)
	var fingerprints []string
	worker.SetIncidentRecorder(func(_ context.Context, _ Rule, status string, _ float64, _ map[string]any, fingerprint string, _ int) (RecordResult, error) {
		if status == "firing" {
			fingerprints = append(fingerprints, fingerprint)
		}
		return RecordResult{ShouldNotify: true}, nil
	})
	rule := testRule("http://example.invalid/webhook", 0)
	rule.Condition = Condition{Type: "any_rows", Operator: "exists"}
	rule.Contacts = nil
	worker.evaluate(context.Background(), rule)

	if len(fingerprints) != 2 {
		t.Fatalf("fingerprints = %#v", fingerprints)
	}
	if fingerprints[0] != "dev:rule-1:svc-a" || fingerprints[1] != "dev:rule-1:svc-b" {
		t.Fatalf("unexpected fingerprints = %#v", fingerprints)
	}
}

func TestEmailContactSkipsWhenSMTPIsNotConfigured(t *testing.T) {
	worker := testWorker(fakeClickHouse(t, `{"value":1}`))
	result := worker.notify(context.Background(), ContactEndpoint{Kind: "email", Target: "alerts@example.com"}, map[string]any{"ruleName": "Email alert"})
	if result.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", result.Status)
	}
}

func TestNotifyPagerDutyEventsV2UsesRoutingKeySecretAndDedupKey(t *testing.T) {
	worker := testWorker(fakeClickHouse(t, `{"value":1}`))
	worker.SetSecretResolver(func(ref string) (string, bool) {
		if ref != "pagerduty-prod-events-key" {
			t.Fatalf("secret ref = %s", ref)
		}
		return "routing-key", true
	})
	worker.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.String() != pagerDutyEventsV2Endpoint {
			t.Fatalf("url = %s", r.URL.String())
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["routing_key"] != "routing-key" || payload["event_action"] != "trigger" || payload["dedup_key"] != "dev:rule-1:svc-a" {
			t.Fatalf("payload = %#v", payload)
		}
		body, _ := payload["payload"].(map[string]any)
		if body["source"] != "checkout" || body["severity"] != "critical" {
			t.Fatalf("event payload = %#v", body)
		}
		if body["custom_details"] == nil {
			t.Fatalf("custom details missing: %#v", body)
		}
		return textResponse(http.StatusAccepted, `{"status":"success"}`), nil
	})}

	result := worker.notify(context.Background(), ContactEndpoint{
		Kind:   "pagerduty",
		Target: "",
		Config: map[string]string{
			"mode":                "events_v2",
			"routingKeySecretRef": "pagerduty-prod-events-key",
			"severity":            "critical",
			"sourceField":         "service_name",
		},
	}, map[string]any{
		"ruleId":      "rule-1",
		"ruleName":    "Timeouts",
		"tenantId":    "dev",
		"fingerprint": "dev:rule-1:svc-a",
		"value":       float64(2),
		"row":         map[string]any{"service_name": "checkout", "message": "timeout"},
	})

	if result.Status != "success" || result.StatusCode != http.StatusAccepted {
		t.Fatalf("result = %#v", result)
	}
}

func fakeClickHouse(t *testing.T, body string) *clickhouse.Client {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		return textResponse(http.StatusOK, body+"\n"), nil
	})}
	return clickhouse.NewClient(config.ClickHouseConfig{
		URL:             "http://clickhouse.local:8123",
		Database:        "default",
		Timeout:         time.Second,
		MaxRows:         100,
		MaxQuerySeconds: 1,
	}, client)
}

func testWorker(ch *clickhouse.Client) *Worker {
	return NewPollingWorker(map[string]config.Dataset{
		"logs": {
			ID:                 "logs",
			Table:              "otel_logs",
			TimeColumn:         "timestamp",
			TenantColumn:       "tenant_id",
			Measures:           []string{"_rows"},
			Aggregations:       []string{"count"},
			DefaultMeasure:     "_rows",
			DefaultAggregation: "count",
		},
	}, 100, ch, func(context.Context) ([]Rule, error) { return nil, nil }, time.Minute, slog.Default())
}

func testRule(webhookURL string, threshold float64) Rule {
	now := time.Now().UTC()
	return Rule{
		ID:       "rule-1",
		Name:     "High log volume",
		TenantID: "dev",
		Query: clickhouse.QueryRequest{
			Dataset: "logs",
			From:    now.Add(-time.Hour),
			To:      now,
		},
		Condition: Condition{Operator: "gt", Threshold: threshold},
		Contacts:  []ContactEndpoint{{Kind: "webhook", Target: webhookURL, Config: map[string]string{}}},
		Enabled:   true,
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
