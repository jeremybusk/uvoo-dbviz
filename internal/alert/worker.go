package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"uvoo-dbviz/internal/clickhouse"
	"uvoo-dbviz/internal/config"
)

type Rule struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	TenantID        string                  `json:"tenantId"`
	Query           clickhouse.QueryRequest `json:"query"`
	Condition       Condition               `json:"condition"`
	IntervalSeconds int                     `json:"intervalSeconds"`
	Contacts        []ContactEndpoint       `json:"contacts"`
	Labels          map[string]string       `json:"labels"`
	Enabled         bool                    `json:"enabled"`
}

type Condition struct {
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	For       string  `json:"for"`
}

type ContactEndpoint struct {
	Kind   string            `json:"kind"`
	Target string            `json:"target"`
	Config map[string]string `json:"config"`
}

type Worker struct {
	datasets map[string]config.Dataset
	maxRows  int
	ch       *clickhouse.Client
	http     *http.Client
	logger   *slog.Logger
	rules    []Rule
	stop     chan struct{}
	wg       sync.WaitGroup
}

func NewWorker(datasets map[string]config.Dataset, maxRows int, ch *clickhouse.Client, rules []Rule, logger *slog.Logger) *Worker {
	return &Worker{
		datasets: datasets,
		maxRows:  maxRows,
		ch:       ch,
		http:     &http.Client{Timeout: 10 * time.Second},
		logger:   logger,
		rules:    rules,
		stop:     make(chan struct{}),
	}
}

func RulesFromJSON(raw string) ([]Rule, error) {
	if raw == "" {
		return nil, nil
	}
	var rules []Rule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].IntervalSeconds <= 0 {
			rules[i].IntervalSeconds = 60
		}
		if rules[i].Condition.Operator == "" {
			rules[i].Condition.Operator = "gt"
		}
		rules[i].Enabled = true
	}
	return rules, nil
}

func (w *Worker) Start(ctx context.Context) {
	for _, rule := range w.rules {
		if !rule.Enabled {
			continue
		}
		rule := rule
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			ticker := time.NewTicker(time.Duration(rule.IntervalSeconds) * time.Second)
			defer ticker.Stop()
			w.evaluate(ctx, rule)
			for {
				select {
				case <-ctx.Done():
					return
				case <-w.stop:
					return
				case <-ticker.C:
					w.evaluate(ctx, rule)
				}
			}
		}()
	}
}

func (w *Worker) Stop() {
	close(w.stop)
	w.wg.Wait()
}

func (w *Worker) evaluate(ctx context.Context, rule Rule) {
	ds, ok := w.datasets[rule.Query.Dataset]
	if !ok {
		w.logger.Warn("alert dataset is unknown", "rule", rule.Name, "dataset", rule.Query.Dataset)
		return
	}
	sql, err := clickhouse.BuildTimeseriesSQL(rule.Query, ds, rule.TenantID, w.maxRows)
	if err != nil {
		w.logger.Warn("alert query rejected", "rule", rule.Name, "error", err)
		return
	}
	rows, err := w.ch.QueryJSONEachRow(ctx, sql)
	if err != nil {
		w.logger.Warn("alert query failed", "rule", rule.Name, "error", err)
		return
	}
	value := maxValue(rows)
	if !compare(value, rule.Condition.Operator, rule.Condition.Threshold) {
		return
	}
	incident := map[string]any{
		"ruleId":    rule.ID,
		"ruleName":  rule.Name,
		"tenantId":  rule.TenantID,
		"value":     value,
		"threshold": rule.Condition.Threshold,
		"operator":  rule.Condition.Operator,
		"labels":    rule.Labels,
		"firedAt":   time.Now().UTC().Format(time.RFC3339),
	}
	for _, contact := range rule.Contacts {
		if err := w.notify(ctx, contact, incident); err != nil {
			w.logger.Warn("alert notification failed", "rule", rule.Name, "kind", contact.Kind, "error", err)
		}
	}
}

func (w *Worker) notify(ctx context.Context, contact ContactEndpoint, incident map[string]any) error {
	switch contact.Kind {
	case "webhook", "pagerduty":
		body, _ := json.Marshal(incident)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, contact.Target, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if token := contact.Config["token"]; token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := w.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("webhook returned %s", resp.Status)
		}
		return nil
	case "email":
		w.logger.Info("email alert contact configured; SMTP sender not enabled yet", "target", contact.Target)
		return nil
	default:
		return fmt.Errorf("unsupported contact kind: %s", contact.Kind)
	}
}

func maxValue(rows []map[string]any) float64 {
	var max float64
	for i, row := range rows {
		value, ok := asFloat(row["value"])
		if !ok {
			continue
		}
		if i == 0 || value > max {
			max = value
		}
	}
	return max
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		var parsed float64
		_, err := fmt.Sscanf(v, "%f", &parsed)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func compare(value float64, op string, threshold float64) bool {
	switch op {
	case "gte":
		return value >= threshold
	case "lt":
		return value < threshold
	case "lte":
		return value <= threshold
	case "eq":
		return value == threshold
	default:
		return value > threshold
	}
}
