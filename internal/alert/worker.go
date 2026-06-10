package alert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	Type      string  `json:"type"`
	Operator  string  `json:"operator"`
	Field     string  `json:"field"`
	Threshold float64 `json:"threshold"`
	Value     string  `json:"value"`
	For       string  `json:"for"`
}

type Evaluation struct {
	Fingerprint string
	Value       float64
	Payload     map[string]any
}

type ContactEndpoint struct {
	Kind   string            `json:"kind"`
	Target string            `json:"target"`
	Config map[string]string `json:"config"`
}

const pagerDutyEventsV2Endpoint = "https://events.pagerduty.com/v2/enqueue"
const pagerDutyRESTAPIEndpoint = "https://api.pagerduty.com"

type Worker struct {
	datasets        map[string]config.Dataset
	maxRows         int
	ch              *clickhouse.Client
	http            *http.Client
	logger          *slog.Logger
	load            func(context.Context) ([]Rule, error)
	record          func(context.Context, Rule, string, float64, map[string]any, string, int) (RecordResult, error)
	delivery        func(context.Context, Rule, string, ContactEndpoint, DeliveryResult, map[string]any) error
	sync            func(context.Context, Rule, string, DeliveryResult) error
	reconcileLoad   func(context.Context) ([]state.PagerDutySyncedIncident, error)
	reconcileRecord func(context.Context, state.PagerDutySyncedIncident, PagerDutyRemoteIncident, DeliveryResult) error
	poll            time.Duration
	dedupe          time.Duration
	smtp            SMTPConfig
	secrets         func(context.Context, string, string) (string, bool)
	pending         map[string]time.Time
	active          map[string]map[string]struct{}
	stop            chan struct{}
	wg              sync.WaitGroup
}

type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

type RecordResult struct {
	IncidentID           string
	Deduped              bool
	ShouldNotify         bool
	ExternalProvider     string
	ExternalIncidentID   string
	ExternalIncidentURL  string
	ExternalSyncStatus   string
	ExternalLastSyncedAt string
}

type DeliveryResult struct {
	Status              string
	StatusCode          int
	Error               string
	ExternalProvider    string
	ExternalIncidentID  string
	ExternalIncidentURL string
	ExternalSyncStatus  string
}

type PagerDutyRemoteIncident struct {
	ID      string
	Status  string
	HTMLURL string
}

func (w *Worker) SetIncidentRecorder(record func(context.Context, Rule, string, float64, map[string]any, string, int) (RecordResult, error)) {
	w.record = record
}

func (w *Worker) SetNotificationRecorder(record func(context.Context, Rule, string, ContactEndpoint, DeliveryResult, map[string]any) error) {
	w.delivery = record
}

func (w *Worker) SetIncidentSyncRecorder(record func(context.Context, Rule, string, DeliveryResult) error) {
	w.sync = record
}

func (w *Worker) SetPagerDutyReconciler(load func(context.Context) ([]state.PagerDutySyncedIncident, error), record func(context.Context, state.PagerDutySyncedIncident, PagerDutyRemoteIncident, DeliveryResult) error) {
	w.reconcileLoad = load
	w.reconcileRecord = record
}

func (w *Worker) SetDedupeWindow(window time.Duration) {
	if window < 0 {
		window = 0
	}
	w.dedupe = window
}

func (w *Worker) SetSMTP(config SMTPConfig) {
	w.smtp = config
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
	if logger == nil {
		logger = slog.Default()
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
		pending:  map[string]time.Time{},
		active:   map[string]map[string]struct{}{},
		secrets:  ResolveSecretRefFromEnv,
		stop:     make(chan struct{}),
	}
}

func (w *Worker) SetSecretResolver(resolve func(context.Context, string, string) (string, bool)) {
	if resolve == nil {
		resolve = ResolveSecretRefFromEnv
	}
	w.secrets = resolve
}

func NewDeliveryTester(smtp SMTPConfig, resolve func(context.Context, string, string) (string, bool), logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	tester := &Worker{
		http:    &http.Client{Timeout: 10 * time.Second},
		logger:  logger,
		smtp:    smtp,
		secrets: ResolveSecretRefFromEnv,
	}
	tester.SetSecretResolver(resolve)
	return tester
}

func (w *Worker) TestContact(ctx context.Context, tenantID string, contact ContactEndpoint) (DeliveryResult, map[string]any) {
	return w.TestContactAction(ctx, tenantID, contact, "trigger")
}

func (w *Worker) TestContactAction(ctx context.Context, tenantID string, contact ContactEndpoint, action string) (DeliveryResult, map[string]any) {
	incident := TestIncidentPayload(tenantID)
	if contact.Kind == "pagerduty" && strings.TrimSpace(action) == "resolve" {
		return w.notifyPagerDuty(ctx, contact, incident, "resolve"), incident
	}
	return w.notify(ctx, contact, incident), incident
}

func TestIncidentPayload(tenantID string) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]any{
		"ruleId":      "contact-test",
		"ruleName":    "DBViz contact test",
		"tenantId":    tenantID,
		"status":      "firing",
		"value":       1,
		"message":     "This is a DBViz test alert notification.",
		"fingerprint": "dbviz-contact-test-" + tenantID,
		"timestamp":   now,
		"row": map[string]any{
			"service_name": "dbviz",
			"severity":     "info",
			"message":      "This is a DBViz test alert notification.",
			"timestamp":    now,
		},
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
		rules[i].Condition = NormalizeCondition(rules[i].Condition)
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
		condition = NormalizeCondition(condition)
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
		w.reconcilePagerDuty(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stop:
				return
			case <-ticker.C:
				w.evaluateDue(ctx, lastEval)
				w.reconcilePagerDuty(ctx)
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

func (w *Worker) reconcilePagerDuty(ctx context.Context) {
	if w.reconcileLoad == nil || w.reconcileRecord == nil {
		return
	}
	incidents, err := w.reconcileLoad(ctx)
	if err != nil {
		w.logger.Warn("PagerDuty reconciliation load failed", "error", err)
		return
	}
	for _, incident := range incidents {
		if strings.TrimSpace(incident.ExternalIncidentID) == "" {
			continue
		}
		remote, result := w.fetchPagerDutyRESTIncident(ctx, incident.TenantID, ContactEndpoint{
			Kind:   "pagerduty",
			Target: incident.ContactTarget,
			Config: incident.ContactConfig,
		}, incident.ExternalIncidentID)
		if err := w.reconcileRecord(ctx, incident, remote, result); err != nil {
			w.logger.Warn("PagerDuty reconciliation record failed", "incident", incident.ID, "remote", incident.ExternalIncidentID, "error", err)
		}
	}
}

func (w *Worker) evaluate(ctx context.Context, rule Rule) {
	ds, ok := w.datasets[rule.Query.Dataset]
	if !ok {
		w.logger.Warn("alert dataset is unknown", "rule", rule.Name, "dataset", rule.Query.Dataset)
		return
	}
	rows, err := w.queryRows(ctx, rule, ds)
	if err != nil {
		return
	}
	baseFingerprint := ruleFingerprint(rule)
	evaluations := EvaluateRows(rule.Condition, rows, baseFingerprint)
	held := map[string]struct{}{}
	current := map[string]struct{}{}
	for _, evaluation := range evaluations {
		current[evaluation.Fingerprint] = struct{}{}
		if !w.conditionHeldLongEnough(rule, evaluation.Fingerprint) {
			w.logger.Info("alert condition is pending", "rule", rule.Name, "for", rule.Condition.For, "fingerprint", evaluation.Fingerprint)
			continue
		}
		held[evaluation.Fingerprint] = struct{}{}
		w.recordAndNotify(ctx, rule, evaluation)
	}
	w.resolveInactive(ctx, rule, baseFingerprint, rows, current)
	if len(held) > 0 {
		w.active[baseFingerprint] = held
	} else if len(current) == 0 {
		delete(w.active, baseFingerprint)
	}
}

func (w *Worker) recordAndNotify(ctx context.Context, rule Rule, evaluation Evaluation) {
	incident := evaluation.Payload
	incident["ruleId"] = rule.ID
	incident["ruleName"] = rule.Name
	incident["tenantId"] = rule.TenantID
	incident["labels"] = rule.Labels
	incident["fingerprint"] = evaluation.Fingerprint
	incident["firedAt"] = time.Now().UTC().Format(time.RFC3339)
	shouldNotify := true
	incidentID := ""
	if w.record != nil {
		result, err := w.record(ctx, rule, "firing", evaluation.Value, incident, evaluation.Fingerprint, int(w.dedupe.Seconds()))
		if err != nil {
			w.logger.Warn("alert incident record failed", "rule", rule.Name, "error", err)
		} else {
			shouldNotify = result.ShouldNotify
			incidentID = result.IncidentID
			incident["incidentId"] = result.IncidentID
			incident["externalProvider"] = result.ExternalProvider
			incident["externalIncidentId"] = result.ExternalIncidentID
			incident["externalIncidentUrl"] = result.ExternalIncidentURL
		}
	}
	if !shouldNotify {
		w.logger.Info("alert notification suppressed by cooldown", "rule", rule.Name, "fingerprint", evaluation.Fingerprint)
		return
	}
	for _, contact := range rule.Contacts {
		result := w.notify(ctx, contact, incident)
		w.recordExternalSync(ctx, rule, incidentID, result)
		if w.delivery != nil {
			if err := w.delivery(ctx, rule, incidentID, contact, result, incident); err != nil {
				w.logger.Warn("alert notification delivery record failed", "rule", rule.Name, "kind", contact.Kind, "error", err)
			}
		}
		if result.Status == "failed" {
			w.logger.Warn("alert notification failed", "rule", rule.Name, "kind", contact.Kind, "error", result.Error)
			if w.record != nil {
				failed := map[string]any{}
				for key, item := range incident {
					failed[key] = item
				}
				failed["contactKind"] = contact.Kind
				failed["contactTarget"] = contact.Target
				failed["statusCode"] = result.StatusCode
				failed["error"] = result.Error
				if _, recordErr := w.record(ctx, rule, "notify_failed", evaluation.Value, failed, evaluation.Fingerprint+":notify:"+contact.Kind+":"+contact.Target, int(w.dedupe.Seconds())); recordErr != nil {
					w.logger.Warn("alert notification failure record failed", "rule", rule.Name, "error", recordErr)
				}
			}
		}
	}
}

func (w *Worker) resolveInactive(ctx context.Context, rule Rule, baseFingerprint string, rows []map[string]any, current map[string]struct{}) {
	previous := w.active[baseFingerprint]
	if len(previous) == 0 && len(current) > 0 {
		return
	}
	resolvedAt := time.Now().UTC().Format(time.RFC3339)
	if len(previous) == 0 {
		delete(w.pending, baseFingerprint)
		w.recordResolved(ctx, rule, baseFingerprint, maxValue(rows, NormalizeCondition(rule.Condition).Field), map[string]any{
			"condition":  NormalizeCondition(rule.Condition),
			"rowCount":   len(rows),
			"resolvedAt": resolvedAt,
		})
		return
	}
	for fingerprint := range previous {
		if _, stillFiring := current[fingerprint]; stillFiring {
			continue
		}
		delete(w.pending, fingerprint)
		w.recordResolved(ctx, rule, fingerprint, 0, map[string]any{
			"condition":  NormalizeCondition(rule.Condition),
			"rowCount":   len(rows),
			"resolvedAt": resolvedAt,
		})
	}
}

func (w *Worker) recordResolved(ctx context.Context, rule Rule, fingerprint string, value float64, payload map[string]any) {
	if w.record == nil {
		return
	}
	payload["ruleId"] = rule.ID
	payload["ruleName"] = rule.Name
	payload["tenantId"] = rule.TenantID
	payload["value"] = value
	payload["labels"] = rule.Labels
	result, err := w.record(ctx, rule, "resolved", value, payload, fingerprint, int(w.dedupe.Seconds()))
	if err != nil {
		w.logger.Warn("alert incident resolve failed", "rule", rule.Name, "error", err)
		return
	}
	if result.IncidentID == "" {
		return
	}
	payload["fingerprint"] = fingerprint
	payload["incidentId"] = result.IncidentID
	payload["externalProvider"] = result.ExternalProvider
	payload["externalIncidentId"] = result.ExternalIncidentID
	payload["externalIncidentUrl"] = result.ExternalIncidentURL
	for _, contact := range rule.Contacts {
		if contact.Kind != "pagerduty" {
			continue
		}
		delivery := w.notifyPagerDuty(ctx, contact, payload, "resolve")
		w.recordExternalSync(ctx, rule, result.IncidentID, delivery)
		if w.delivery != nil {
			if err := w.delivery(ctx, rule, result.IncidentID, contact, delivery, payload); err != nil {
				w.logger.Warn("alert resolve notification delivery record failed", "rule", rule.Name, "kind", contact.Kind, "error", err)
			}
		}
	}
}

func (w *Worker) recordExternalSync(ctx context.Context, rule Rule, incidentID string, result DeliveryResult) {
	if w.sync == nil || incidentID == "" || result.ExternalProvider == "" {
		return
	}
	if err := w.sync(ctx, rule, incidentID, result); err != nil {
		w.logger.Warn("alert incident external sync record failed", "rule", rule.Name, "provider", result.ExternalProvider, "error", err)
	}
}

func (w *Worker) conditionHeldLongEnough(rule Rule, fingerprint string) bool {
	holdFor, err := time.ParseDuration(rule.Condition.For)
	if rule.Condition.For == "" {
		return true
	}
	if err != nil || holdFor <= 0 {
		return true
	}
	now := time.Now()
	since, ok := w.pending[fingerprint]
	if !ok {
		w.pending[fingerprint] = now
		return false
	}
	return now.Sub(since) >= holdFor
}

func (w *Worker) queryRows(ctx context.Context, rule Rule, ds config.Dataset) ([]map[string]any, error) {
	if rule.Query.Mode == "sql" {
		built, err := clickhouse.BuildCustomSQL(rule.Query, ds, rule.TenantID, w.maxRows, clickhouse.CustomSQLAlert)
		if err != nil {
			w.logger.Warn("alert custom sql rejected", "rule", rule.Name, "error", err)
			return nil, err
		}
		rows, err := w.ch.QueryJSONEachRowWithParams(ctx, built.SQL, built.Params)
		if err != nil {
			w.logger.Warn("alert custom sql failed", "rule", rule.Name, "error", err)
			return nil, err
		}
		return rows, nil
	}
	sql, err := clickhouse.BuildTimeseriesSQL(rule.Query, ds, rule.TenantID, w.maxRows)
	if err != nil {
		w.logger.Warn("alert query rejected", "rule", rule.Name, "error", err)
		return nil, err
	}
	rows, err := w.ch.QueryJSONEachRow(ctx, sql)
	if err != nil {
		w.logger.Warn("alert query failed", "rule", rule.Name, "error", err)
		return nil, err
	}
	return rows, nil
}

func ruleFingerprint(rule Rule) string {
	if rule.ID != "" {
		return rule.TenantID + ":" + rule.ID
	}
	if rule.Query.Mode == "sql" {
		return rule.TenantID + ":" + rule.Name + ":" + rule.Query.Dataset + ":sql"
	}
	return rule.TenantID + ":" + rule.Name + ":" + rule.Query.Dataset + ":" + rule.Query.GroupBy
}

func (w *Worker) notify(ctx context.Context, contact ContactEndpoint, incident map[string]any) DeliveryResult {
	switch contact.Kind {
	case "pagerduty":
		return w.notifyPagerDuty(ctx, contact, incident, "trigger")
	case "webhook":
		body, contentType, err := w.webhookBody(contact, incident)
		if err != nil {
			return DeliveryResult{Status: "failed", Error: err.Error()}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, contact.Target, bytes.NewReader(body))
		if err != nil {
			return DeliveryResult{Status: "failed", Error: err.Error()}
		}
		req.Header.Set("Content-Type", contentType)
		tenantID := strings.TrimSpace(fmt.Sprint(incident["tenantId"]))
		if token, err := w.webhookSecretValue(ctx, tenantID, contact, "tokenSecretRef", "token"); err != nil {
			return DeliveryResult{Status: "failed", Error: err.Error()}
		} else if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if headerName := strings.TrimSpace(contact.Config["headerName"]); headerName != "" {
			if headerValue, err := w.webhookSecretValue(ctx, tenantID, contact, "headerValueSecretRef", "headerValue"); err != nil {
				return DeliveryResult{Status: "failed", Error: err.Error()}
			} else if headerValue != "" {
				req.Header.Set(headerName, headerValue)
			}
		}
		resp, err := w.http.Do(req)
		if err != nil {
			return DeliveryResult{Status: "failed", Error: err.Error()}
		}
		defer resp.Body.Close()
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= 300 {
			errText := fmt.Sprintf("webhook returned %s", resp.Status)
			if len(responseBody) > 0 {
				errText = errText + ": " + string(responseBody)
			}
			return DeliveryResult{Status: "failed", StatusCode: resp.StatusCode, Error: errText}
		}
		return DeliveryResult{Status: "success", StatusCode: resp.StatusCode}
	case "email":
		return w.notifyEmail(contact, incident)
	default:
		return DeliveryResult{Status: "failed", Error: fmt.Sprintf("unsupported contact kind: %s", contact.Kind)}
	}
}

func (w *Worker) webhookBody(contact ContactEndpoint, incident map[string]any) ([]byte, string, error) {
	template := strings.TrimSpace(contact.Config["bodyTemplate"])
	if template == "" {
		body, _ := json.Marshal(incident)
		return body, "application/json", nil
	}
	rendered := renderTemplate(template, incident)
	if json.Valid([]byte(rendered)) {
		return []byte(rendered), "application/json", nil
	}
	return []byte(rendered), "text/plain; charset=utf-8", nil
}

func (w *Worker) webhookSecretValue(ctx context.Context, tenantID string, contact ContactEndpoint, refKey string, inlineKey string) (string, error) {
	if value := strings.TrimSpace(contact.Config[inlineKey]); value != "" {
		return value, nil
	}
	ref := strings.TrimSpace(contact.Config[refKey])
	if ref == "" {
		return "", nil
	}
	value, ok := w.secrets(ctx, tenantID, ref)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("webhook secret ref is not configured: %s", ref)
	}
	return strings.TrimSpace(value), nil
}

func (w *Worker) notifyEmail(contact ContactEndpoint, incident map[string]any) DeliveryResult {
	if w.smtp.Host == "" || w.smtp.From == "" {
		w.logger.Info("email alert contact configured; SMTP sender not enabled yet", "target", contact.Target)
		return DeliveryResult{Status: "skipped", Error: "email delivery is not configured"}
	}
	port := w.smtp.Port
	if port <= 0 {
		port = 587
	}
	addr := net.JoinHostPort(w.smtp.Host, fmt.Sprint(port))
	body, _ := json.MarshalIndent(incident, "", "  ")
	subject := fmt.Sprintf("DBViz alert: %v", incident["ruleName"])
	message := strings.Join([]string{
		"From: " + w.smtp.From,
		"To: " + contact.Target,
		"Subject: " + subject,
		"Content-Type: application/json; charset=utf-8",
		"",
		string(body),
	}, "\r\n")
	var auth smtp.Auth
	if w.smtp.User != "" {
		auth = smtp.PlainAuth("", w.smtp.User, w.smtp.Password, w.smtp.Host)
	}
	if err := smtp.SendMail(addr, auth, w.smtp.From, []string{contact.Target}, []byte(message)); err != nil {
		return DeliveryResult{Status: "failed", Error: err.Error()}
	}
	return DeliveryResult{Status: "success"}
}

func (w *Worker) notifyPagerDuty(ctx context.Context, contact ContactEndpoint, incident map[string]any, action string) DeliveryResult {
	if pagerDutyRESTSyncEnabled(contact) {
		return w.notifyPagerDutyREST(ctx, contact, incident, action)
	}
	mode := strings.TrimSpace(contact.Config["mode"])
	if mode == "" {
		mode = "events_v2"
	}
	if mode != "events_v2" {
		return DeliveryResult{Status: "failed", Error: fmt.Sprintf("unsupported PagerDuty mode: %s", mode)}
	}
	tenantID := strings.TrimSpace(fmt.Sprint(incident["tenantId"]))
	routingKey, err := w.pagerDutyRoutingKey(ctx, tenantID, contact)
	if err != nil {
		return DeliveryResult{Status: "failed", Error: err.Error()}
	}
	endpoint := strings.TrimSpace(contact.Target)
	if endpoint == "" || endpoint == "events_v2" {
		endpoint = pagerDutyEventsV2Endpoint
	}
	payload := pagerDutyEventPayload(contact, incident, routingKey, action)
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return DeliveryResult{Status: "failed", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return DeliveryResult{Status: "failed", Error: err.Error()}
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		errText := fmt.Sprintf("PagerDuty returned %s", resp.Status)
		if len(responseBody) > 0 {
			errText = errText + ": " + string(responseBody)
		}
		return DeliveryResult{Status: "failed", StatusCode: resp.StatusCode, Error: errText}
	}
	return DeliveryResult{Status: "success", StatusCode: resp.StatusCode}
}

func (w *Worker) notifyPagerDutyREST(ctx context.Context, contact ContactEndpoint, incident map[string]any, action string) DeliveryResult {
	tenantID := strings.TrimSpace(fmt.Sprint(incident["tenantId"]))
	apiKey, err := w.pagerDutyRESTAPIKey(ctx, tenantID, contact)
	if err != nil {
		return DeliveryResult{Status: "failed", Error: err.Error(), ExternalProvider: "pagerduty", ExternalSyncStatus: "failed"}
	}
	from := strings.TrimSpace(contact.Config["fromEmail"])
	if from == "" {
		return DeliveryResult{Status: "failed", Error: "PagerDuty REST fromEmail is required", ExternalProvider: "pagerduty", ExternalSyncStatus: "failed"}
	}
	if action == "resolve" {
		remoteID := firstNonEmptyString(fmt.Sprint(incident["externalIncidentId"]), fmt.Sprint(incident["pagerDutyIncidentId"]))
		if remoteID == "" {
			return DeliveryResult{Status: "skipped", Error: "PagerDuty REST incident id is not available", ExternalProvider: "pagerduty", ExternalSyncStatus: "skipped"}
		}
		return w.updatePagerDutyRESTIncident(ctx, contact, apiKey, from, remoteID, "resolved")
	}
	if remoteID := firstNonEmptyString(fmt.Sprint(incident["externalIncidentId"]), fmt.Sprint(incident["pagerDutyIncidentId"])); remoteID != "" {
		return DeliveryResult{
			Status:              "success",
			ExternalProvider:    "pagerduty",
			ExternalIncidentID:  remoteID,
			ExternalIncidentURL: firstNonEmptyString(fmt.Sprint(incident["externalIncidentUrl"]), fmt.Sprint(incident["pagerDutyIncidentUrl"])),
			ExternalSyncStatus:  "existing",
		}
	}
	return w.createPagerDutyRESTIncident(ctx, contact, incident, apiKey, from)
}

func (w *Worker) createPagerDutyRESTIncident(ctx context.Context, contact ContactEndpoint, incident map[string]any, apiKey string, from string) DeliveryResult {
	serviceID := strings.TrimSpace(contact.Config["serviceId"])
	if serviceID == "" {
		return DeliveryResult{Status: "failed", Error: "PagerDuty REST serviceId is required", ExternalProvider: "pagerduty", ExternalSyncStatus: "failed"}
	}
	eventPayload := pagerDutyCEF(contact, incident)
	details, _ := json.MarshalIndent(incident, "", "  ")
	body := map[string]any{
		"incident": map[string]any{
			"type":  "incident",
			"title": eventPayload["summary"],
			"service": map[string]any{
				"id":   serviceID,
				"type": "service_reference",
			},
			"body": map[string]any{
				"type":    "incident_body",
				"details": string(details),
			},
		},
	}
	if urgency := pagerDutyUrgency(contact.Config["severity"]); urgency != "" {
		body["incident"].(map[string]any)["urgency"] = urgency
	}
	respBody, statusCode, err := w.pagerDutyRESTRequest(ctx, contact, apiKey, from, http.MethodPost, "/incidents", body)
	if err != nil {
		return DeliveryResult{Status: "failed", StatusCode: statusCode, Error: err.Error(), ExternalProvider: "pagerduty", ExternalSyncStatus: "failed"}
	}
	remoteID, remoteURL := pagerDutyIncidentResponseFields(respBody)
	return DeliveryResult{
		Status:              "success",
		StatusCode:          statusCode,
		ExternalProvider:    "pagerduty",
		ExternalIncidentID:  remoteID,
		ExternalIncidentURL: remoteURL,
		ExternalSyncStatus:  "created",
	}
}

func (w *Worker) updatePagerDutyRESTIncident(ctx context.Context, contact ContactEndpoint, apiKey string, from string, remoteID string, status string) DeliveryResult {
	body := map[string]any{
		"incident": map[string]any{
			"type":   "incident",
			"status": status,
		},
	}
	respBody, statusCode, err := w.pagerDutyRESTRequest(ctx, contact, apiKey, from, http.MethodPut, "/incidents/"+url.PathEscape(remoteID), body)
	if err != nil {
		return DeliveryResult{Status: "failed", StatusCode: statusCode, Error: err.Error(), ExternalProvider: "pagerduty", ExternalIncidentID: remoteID, ExternalSyncStatus: "failed"}
	}
	parsedID, remoteURL := pagerDutyIncidentResponseFields(respBody)
	if parsedID == "" {
		parsedID = remoteID
	}
	return DeliveryResult{
		Status:              "success",
		StatusCode:          statusCode,
		ExternalProvider:    "pagerduty",
		ExternalIncidentID:  parsedID,
		ExternalIncidentURL: remoteURL,
		ExternalSyncStatus:  status,
	}
}

func (w *Worker) fetchPagerDutyRESTIncident(ctx context.Context, tenantID string, contact ContactEndpoint, remoteID string) (PagerDutyRemoteIncident, DeliveryResult) {
	apiKey, err := w.pagerDutyRESTAPIKey(ctx, tenantID, contact)
	if err != nil {
		return PagerDutyRemoteIncident{ID: remoteID}, DeliveryResult{Status: "failed", Error: err.Error(), ExternalProvider: "pagerduty", ExternalIncidentID: remoteID, ExternalSyncStatus: "reconcile_failed"}
	}
	respBody, statusCode, err := w.pagerDutyRESTRequest(ctx, contact, apiKey, strings.TrimSpace(contact.Config["fromEmail"]), http.MethodGet, "/incidents/"+url.PathEscape(remoteID), nil)
	if err != nil {
		return PagerDutyRemoteIncident{ID: remoteID}, DeliveryResult{Status: "failed", StatusCode: statusCode, Error: err.Error(), ExternalProvider: "pagerduty", ExternalIncidentID: remoteID, ExternalSyncStatus: "reconcile_failed"}
	}
	remote := pagerDutyIncidentResponse(respBody)
	if remote.ID == "" {
		remote.ID = remoteID
	}
	return remote, DeliveryResult{
		Status:              "success",
		StatusCode:          statusCode,
		ExternalProvider:    "pagerduty",
		ExternalIncidentID:  remote.ID,
		ExternalIncidentURL: remote.HTMLURL,
		ExternalSyncStatus:  "remote_" + firstNonEmptyString(remote.Status, "unknown"),
	}
}

func (w *Worker) pagerDutyRESTRequest(ctx context.Context, contact ContactEndpoint, apiKey string, from string, method string, path string, payload map[string]any) ([]byte, int, error) {
	baseURL := strings.TrimRight(firstNonEmptyString(contact.Config["apiBaseURL"], pagerDutyRESTAPIEndpoint), "/")
	var reader io.Reader
	if payload != nil {
		body, _ := json.Marshal(payload)
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Token token="+apiKey)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if from != "" {
		req.Header.Set("From", from)
	}
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if resp.StatusCode >= 300 {
		errText := fmt.Sprintf("PagerDuty REST returned %s", resp.Status)
		if len(responseBody) > 0 {
			errText += ": " + string(responseBody)
		}
		return responseBody, resp.StatusCode, errors.New(errText)
	}
	return responseBody, resp.StatusCode, nil
}

func pagerDutyIncidentResponseFields(body []byte) (string, string) {
	incident := pagerDutyIncidentResponse(body)
	return incident.ID, incident.HTMLURL
}

func pagerDutyIncidentResponse(body []byte) PagerDutyRemoteIncident {
	var parsed struct {
		Incident struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			HTMLURL string `json:"html_url"`
		} `json:"incident"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return PagerDutyRemoteIncident{}
	}
	return PagerDutyRemoteIncident{
		ID:      strings.TrimSpace(parsed.Incident.ID),
		Status:  strings.TrimSpace(parsed.Incident.Status),
		HTMLURL: strings.TrimSpace(parsed.Incident.HTMLURL),
	}
}

func pagerDutyRESTSyncEnabled(contact ContactEndpoint) bool {
	value := strings.ToLower(strings.TrimSpace(contact.Config["restSyncEnabled"]))
	return value == "true" || value == "1" || value == "yes"
}

func pagerDutyUrgency(severity string) string {
	switch strings.TrimSpace(severity) {
	case "critical", "error":
		return "high"
	case "warning", "info":
		return "low"
	default:
		return ""
	}
}

func (w *Worker) pagerDutyRESTAPIKey(ctx context.Context, tenantID string, contact ContactEndpoint) (string, error) {
	if value := strings.TrimSpace(contact.Config["restApiKey"]); value != "" {
		return value, nil
	}
	ref := strings.TrimSpace(contact.Config["restApiKeySecretRef"])
	if ref == "" {
		return "", errors.New("PagerDuty REST restApiKeySecretRef is required")
	}
	value, ok := w.secrets(ctx, tenantID, ref)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("PagerDuty REST API key secret ref is not configured: %s", ref)
	}
	return strings.TrimSpace(value), nil
}

func (w *Worker) pagerDutyRoutingKey(ctx context.Context, tenantID string, contact ContactEndpoint) (string, error) {
	if value := strings.TrimSpace(contact.Config["routingKey"]); value != "" {
		return value, nil
	}
	ref := strings.TrimSpace(contact.Config["routingKeySecretRef"])
	if ref == "" {
		return "", errors.New("PagerDuty routingKeySecretRef is required")
	}
	value, ok := w.secrets(ctx, tenantID, ref)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("PagerDuty routing key secret ref is not configured: %s", ref)
	}
	return strings.TrimSpace(value), nil
}

func pagerDutyEventPayload(contact ContactEndpoint, incident map[string]any, routingKey string, action string) map[string]any {
	if action != "resolve" && action != "acknowledge" {
		action = "trigger"
	}
	dedupKey := strings.TrimSpace(fmt.Sprint(incident["fingerprint"]))
	if dedupKey == "" || dedupKey == "<nil>" {
		dedupKey = strings.TrimSpace(fmt.Sprint(incident["ruleId"]))
	}
	payload := map[string]any{
		"routing_key":  routingKey,
		"event_action": action,
		"dedup_key":    dedupKey,
	}
	if action == "trigger" {
		eventPayload := pagerDutyCEF(contact, incident)
		eventPayload["custom_details"] = incident
		payload["payload"] = eventPayload
	}
	return payload
}

func pagerDutyCEF(contact ContactEndpoint, incident map[string]any) map[string]any {
	row, _ := incident["row"].(map[string]any)
	summary := firstNonEmptyString(
		contact.Config["summary"],
		fmt.Sprint(incident["message"]),
		fmt.Sprint(row["message"]),
		fmt.Sprintf("%v fired with value %v", incident["ruleName"], incident["value"]),
	)
	source := firstNonEmptyString(
		valueFromField(row, contact.Config["sourceField"]),
		fmt.Sprint(row["service_name"]),
		fmt.Sprint(incident["tenantId"]),
		"uvoo-dbviz",
	)
	severity := strings.TrimSpace(contact.Config["severity"])
	if severity == "" {
		severity = "error"
	}
	return map[string]any{
		"summary":   summary,
		"source":    source,
		"severity":  severity,
		"component": firstNonEmptyString(contact.Config["component"], fmt.Sprint(row["service_name"]), "uvoo-dbviz"),
		"group":     firstNonEmptyString(contact.Config["group"], fmt.Sprint(incident["tenantId"]), "observability"),
		"class":     firstNonEmptyString(contact.Config["class"], fmt.Sprint(incident["ruleName"]), "alert"),
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func valueFromField(row map[string]any, field string) string {
	field = strings.TrimSpace(field)
	if field == "" || row == nil {
		return ""
	}
	return fmt.Sprint(row[field])
}

var templateTokenPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

func renderTemplate(template string, data map[string]any) string {
	return templateTokenPattern.ReplaceAllStringFunc(template, func(token string) string {
		matches := templateTokenPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			return ""
		}
		value, ok := nestedTemplateValue(data, strings.Split(matches[1], "."))
		if !ok || value == nil {
			return ""
		}
		return fmt.Sprint(value)
	})
}

func nestedTemplateValue(data map[string]any, path []string) (any, bool) {
	var current any = data
	for _, part := range path {
		if part == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func ResolveSecretRefFromEnv(_ context.Context, _ string, ref string) (string, bool) {
	name := SecretEnvName(ref)
	value, ok := os.LookupEnv(name)
	return value, ok
}

var secretRefCleaner = regexp.MustCompile(`[^A-Za-z0-9]+`)

func SecretEnvName(ref string) string {
	clean := strings.Trim(secretRefCleaner.ReplaceAllString(strings.ToUpper(strings.TrimSpace(ref)), "_"), "_")
	return "DBVIZ_SECRET_" + clean
}

func NormalizeCondition(condition Condition) Condition {
	if condition.Type == "" {
		condition.Type = "numeric_threshold"
	}
	switch condition.Type {
	case "numeric_threshold", "row_count", "any_rows", "sql_result", "no_data", "text_match":
	default:
		condition.Type = "numeric_threshold"
	}
	if condition.Operator == "" {
		switch condition.Type {
		case "text_match":
			condition.Operator = "contains"
		default:
			condition.Operator = "gt"
		}
	}
	if condition.Field == "" {
		switch condition.Type {
		case "text_match":
			condition.Field = "message"
		default:
			condition.Field = "value"
		}
	}
	return condition
}

func EvaluateRows(condition Condition, rows []map[string]any, baseFingerprint string) []Evaluation {
	condition = NormalizeCondition(condition)
	switch condition.Type {
	case "any_rows", "sql_result":
		evaluations := make([]Evaluation, 0, len(rows))
		for index, row := range rows {
			evaluations = append(evaluations, rowEvaluation(condition, row, baseFingerprint, index))
		}
		return evaluations
	case "no_data":
		if len(rows) == 0 {
			return []Evaluation{{
				Fingerprint: baseFingerprint,
				Value:       0,
				Payload:     basePayload(condition, 0),
			}}
		}
		return nil
	case "row_count":
		value := float64(len(rows))
		if compare(value, condition.Operator, condition.Threshold) {
			payload := basePayload(condition, value)
			payload["rowCount"] = len(rows)
			return []Evaluation{{Fingerprint: baseFingerprint, Value: value, Payload: payload}}
		}
		return nil
	case "text_match":
		evaluations := []Evaluation{}
		for index, row := range rows {
			text := rowText(row, condition.Field)
			if compareText(text, condition.Operator, condition.Value) {
				evaluation := rowEvaluation(condition, row, baseFingerprint, index)
				evaluation.Payload["text"] = text
				evaluations = append(evaluations, evaluation)
			}
		}
		return evaluations
	default:
		value := maxValue(rows, condition.Field)
		if compare(value, condition.Operator, condition.Threshold) {
			return []Evaluation{{
				Fingerprint: baseFingerprint,
				Value:       value,
				Payload:     basePayload(condition, value),
			}}
		}
		return nil
	}
}

func basePayload(condition Condition, value float64) map[string]any {
	return map[string]any{
		"value":     value,
		"condition": condition,
		"threshold": condition.Threshold,
		"operator":  condition.Operator,
		"field":     condition.Field,
	}
}

func rowEvaluation(condition Condition, row map[string]any, baseFingerprint string, index int) Evaluation {
	value, ok := asFloat(row[condition.Field])
	if condition.Field != "value" && !ok {
		value, _ = asFloat(row["value"])
	}
	fingerprint := rowFingerprint(row, baseFingerprint, index)
	payload := basePayload(condition, value)
	payload["row"] = row
	if message := rowText(row, "message"); message != "" {
		payload["message"] = message
	}
	return Evaluation{Fingerprint: fingerprint, Value: value, Payload: payload}
}

func rowFingerprint(row map[string]any, baseFingerprint string, index int) string {
	if value := strings.TrimSpace(fmt.Sprint(row["fingerprint"])); value != "" && value != "<nil>" {
		return baseFingerprint + ":" + value
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return fmt.Sprintf("%s:row:%d", baseFingerprint, index)
	}
	sum := sha256.Sum256(encoded)
	return baseFingerprint + ":row:" + hex.EncodeToString(sum[:8])
}

func rowText(row map[string]any, field string) string {
	if field != "" {
		if value, ok := row[field]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		switch value := row[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, " ")
}

func compareText(text string, op string, pattern string) bool {
	switch op {
	case "eq":
		return text == pattern
	case "neq":
		return text != pattern
	case "not_contains":
		return !strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
	case "regex":
		matched, err := regexp.MatchString(pattern, text)
		return err == nil && matched
	default:
		return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
	}
}

func maxValue(rows []map[string]any, field string) float64 {
	if field == "" {
		field = "value"
	}
	var max float64
	for i, row := range rows {
		value, ok := asFloat(row[field])
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
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
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
	case "neq":
		return value != threshold
	default:
		return value > threshold
	}
}
