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
	"uvoo-dbviz/internal/state"
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
	load     func(context.Context) ([]Rule, error)
	record   func(context.Context, Rule, string, float64, map[string]any, string, int) (RecordResult, error)
	poll     time.Duration
	dedupe   time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
}

type RecordResult struct {
	Deduped      bool
	ShouldNotify bool
}

func (w *Worker) SetIncidentRecorder(record func(context.Context, Rule, string, float64, map[string]any, string, int) (RecordResult, error)) {
	w.record = record
}

func (w *Worker) SetDedupeWindow(window time.Duration) {
	if window < 0 {
		window = 0
	}
	w.dedupe = window
}

func NewWorker(datasets map[string]config.Dataset, maxRows int, ch *clickhouse.Client, rules []Rule, logger *slog.Logger) *Worker {
	return NewPollingWorker(datasets, maxRows, ch, func(context.Context) ([]Rule, error) {
		return rules, nil
	}, time.Minute, logger)
}

func NewPollingWorker(datasets map[string]config.Dataset, maxRows int, ch *clickhouse.Client, load func(context.Context) ([]Rule, error), poll time.Duration, logger *slog.Logger) *Worker {
	if poll <= 0 {
		poll = time.Minute
	}
	return &Worker{
		datasets: datasets,
		maxRows:  maxRows,
		ch:       ch,
		http:     &http.Client{Timeout: 10 * time.Second},
		logger:   logger,
		load:     load,
		poll:     poll,
		dedupe:   5 * time.Minute,
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

func RulesFromPersisted(rows []state.PersistedAlertRule) ([]Rule, error) {
	rules := make([]Rule, 0, len(rows))
	for _, row := range rows {
		queryBytes, err := json.Marshal(row.Query)
		if err != nil {
			return nil, err
		}
		var query clickhouse.QueryRequest
		if err := json.Unmarshal(queryBytes, &query); err != nil {
			return nil, err
		}
		conditionBytes, err := json.Marshal(row.Condition)
		if err != nil {
			return nil, err
		}
		var condition Condition
		if err := json.Unmarshal(conditionBytes, &condition); err != nil {
			return nil, err
		}
		if condition.Operator == "" {
			condition.Operator = "gt"
		}
		rule := Rule{
			ID:              row.ID,
			Name:            row.Name,
			TenantID:        row.TenantID,
			Query:           query,
			Condition:       condition,
			IntervalSeconds: row.IntervalSeconds,
			Enabled:         row.Enabled,
		}
		if row.ContactKind != "" && row.ContactTarget != "" {
			rule.Contacts = []ContactEndpoint{{
				Kind:   row.ContactKind,
				Target: row.ContactTarget,
				Config: row.ContactConfig,
			}}
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.poll)
		defer ticker.Stop()
		lastEval := map[string]time.Time{}
		w.evaluateDue(ctx, lastEval)
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stop:
				return
			case <-ticker.C:
				w.evaluateDue(ctx, lastEval)
			}
		}
	}()
}

func (w *Worker) Stop() {
	close(w.stop)
	w.wg.Wait()
}

func (w *Worker) evaluateDue(ctx context.Context, lastEval map[string]time.Time) {
	rules, err := w.load(ctx)
	if err != nil {
		w.logger.Warn("alert rule load failed", "error", err)
		return
	}
	now := time.Now()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		interval := time.Duration(rule.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = time.Minute
		}
		key := rule.ID
		if key == "" {
			key = rule.Name + ":" + rule.TenantID
		}
		if last, ok := lastEval[key]; ok && now.Sub(last) < interval {
			continue
		}
		lastEval[key] = now
		w.evaluate(ctx, rule)
	}
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
	fingerprint := ruleFingerprint(rule)
	if !compare(value, rule.Condition.Operator, rule.Condition.Threshold) {
		if w.record != nil {
			resolved := map[string]any{
				"ruleId":     rule.ID,
				"ruleName":   rule.Name,
				"tenantId":   rule.TenantID,
				"value":      value,
				"threshold":  rule.Condition.Threshold,
				"operator":   rule.Condition.Operator,
				"labels":     rule.Labels,
				"resolvedAt": time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := w.record(ctx, rule, "resolved", value, resolved, fingerprint, int(w.dedupe.Seconds())); err != nil {
				w.logger.Warn("alert incident resolve failed", "rule", rule.Name, "error", err)
			}
		}
		return
	}
	incident := map[string]any{
		"ruleId":      rule.ID,
		"ruleName":    rule.Name,
		"tenantId":    rule.TenantID,
		"value":       value,
		"threshold":   rule.Condition.Threshold,
		"operator":    rule.Condition.Operator,
		"labels":      rule.Labels,
		"fingerprint": fingerprint,
		"firedAt":     time.Now().UTC().Format(time.RFC3339),
	}
	shouldNotify := true
	if w.record != nil {
		result, err := w.record(ctx, rule, "firing", value, incident, fingerprint, int(w.dedupe.Seconds()))
		if err != nil {
			w.logger.Warn("alert incident record failed", "rule", rule.Name, "error", err)
		} else {
			shouldNotify = result.ShouldNotify
		}
	}
	if !shouldNotify {
		w.logger.Info("alert notification suppressed by cooldown", "rule", rule.Name, "fingerprint", fingerprint)
		return
	}
	for _, contact := range rule.Contacts {
		if err := w.notify(ctx, contact, incident); err != nil {
			w.logger.Warn("alert notification failed", "rule", rule.Name, "kind", contact.Kind, "error", err)
			if w.record != nil {
				failed := map[string]any{}
				for key, item := range incident {
					failed[key] = item
				}
				failed["contactKind"] = contact.Kind
				failed["contactTarget"] = contact.Target
				failed["error"] = err.Error()
				if _, recordErr := w.record(ctx, rule, "notify_failed", value, failed, fingerprint+":notify:"+contact.Kind+":"+contact.Target, int(w.dedupe.Seconds())); recordErr != nil {
					w.logger.Warn("alert notification failure record failed", "rule", rule.Name, "error", recordErr)
				}
			}
		}
	}
}

func ruleFingerprint(rule Rule) string {
	if rule.ID != "" {
		return rule.TenantID + ":" + rule.ID
	}
	return rule.TenantID + ":" + rule.Name + ":" + rule.Query.Dataset + ":" + rule.Query.GroupBy
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
