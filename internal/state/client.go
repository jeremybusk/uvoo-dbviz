package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"uvoo-dbviz/internal/auth"
	"uvoo-dbviz/internal/config"
)

type Client struct {
	baseURL       string
	forwardBearer bool
	http          *http.Client
}

type PersistedAlertRule struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	TenantID        string            `json:"tenant_id"`
	Query           map[string]any    `json:"query"`
	Condition       map[string]any    `json:"condition"`
	IntervalSeconds int               `json:"interval_seconds"`
	Enabled         bool              `json:"enabled"`
	ContactKind     string            `json:"contact_kind"`
	ContactTarget   string            `json:"contact_target"`
	ContactConfig   map[string]string `json:"contact_config"`
}

type TenantSecret struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	KeyVersion  string `json:"key_version"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type TenantSecretUsage struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	ResourceName string `json:"resource_name"`
	Field        string `json:"field"`
}

type EncryptedTenantSecret struct {
	Name       string `json:"name"`
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
	KeyVersion string `json:"key_version"`
}

type UserProfile struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	TenantSlug  string `json:"tenant_slug"`
	Subject     string `json:"subject"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Role        string `json:"role"`
}

type DataSource struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Config    map[string]any `json:"config"`
	UpdatedAt string         `json:"updated_at"`
	CreatedAt string         `json:"created_at"`
}

type AlertIncident struct {
	ID                   string         `json:"id"`
	AlertRuleID          *string        `json:"alert_rule_id"`
	Fingerprint          string         `json:"fingerprint"`
	Status               string         `json:"status"`
	Value                float64        `json:"value"`
	Payload              map[string]any `json:"payload"`
	OccurrenceCount      int            `json:"occurrence_count"`
	FirstSeenAt          string         `json:"first_seen_at"`
	LastSeenAt           string         `json:"last_seen_at"`
	LastNotifiedAt       *string        `json:"last_notified_at"`
	ResolvedAt           *string        `json:"resolved_at"`
	ExternalProvider     string         `json:"external_provider"`
	ExternalIncidentID   string         `json:"external_incident_id"`
	ExternalIncidentURL  string         `json:"external_incident_url"`
	ExternalSyncStatus   string         `json:"external_sync_status"`
	ExternalSyncError    string         `json:"external_sync_error"`
	ExternalLastSyncedAt *string        `json:"external_last_synced_at"`
	CreatedAt            string         `json:"created_at"`
	Deduped              bool           `json:"deduped,omitempty"`
	ShouldNotify         bool           `json:"should_notify,omitempty"`
}

type AlertNotification struct {
	ID              string         `json:"id"`
	AlertRuleID     *string        `json:"alert_rule_id"`
	AlertIncidentID *string        `json:"alert_incident_id"`
	ContactKind     string         `json:"contact_kind"`
	ContactTarget   string         `json:"contact_target"`
	Status          string         `json:"status"`
	StatusCode      int            `json:"status_code"`
	Error           string         `json:"error"`
	Payload         map[string]any `json:"payload"`
	CreatedAt       string         `json:"created_at"`
}

type PagerDutySyncedIncident struct {
	ID                  string            `json:"id"`
	TenantID            string            `json:"tenant_id"`
	AlertRuleID         *string           `json:"alert_rule_id"`
	Fingerprint         string            `json:"fingerprint"`
	Status              string            `json:"status"`
	ExternalIncidentID  string            `json:"external_incident_id"`
	ExternalIncidentURL string            `json:"external_incident_url"`
	ContactTarget       string            `json:"contact_target"`
	ContactConfig       map[string]string `json:"contact_config"`
}

func NewClient(cfg config.PostgRESTConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(cfg.URL, "/"), forwardBearer: cfg.ForwardBearer, http: httpClient}
}

func (c *Client) Enabled() bool {
	return c.baseURL != ""
}

func (c *Client) RPC(ctx context.Context, name string, body any, user auth.Principal, bearer string, out any) error {
	if !c.Enabled() {
		return fmt.Errorf("postgrest is not configured")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rpc/"+name, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DBViz-Tenant", user.TenantID)
	req.Header.Set("X-DBViz-Subject", user.Subject)
	req.Header.Set("X-DBViz-Provider", user.Provider)
	req.Header.Set("X-DBViz-Email", user.Email)
	if bearer != "" && c.forwardBearer {
		req.Header.Set("Authorization", bearer)
	} else {
		req.Header.Set("X-Dev-Tenant", user.TenantID)
		req.Header.Set("X-Dev-Email", user.Email)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("postgrest rpc %s returned %s: %s", name, resp.Status, strings.TrimSpace(string(responseBody)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(responseBody, out)
}

func (c *Client) LoadEnabledAlertRules(ctx context.Context, workerKey string) ([]PersistedAlertRule, error) {
	var rows []PersistedAlertRule
	err := c.RPC(ctx, "list_enabled_alert_rules_for_worker", map[string]any{
		"worker_key": workerKey,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	return rows, err
}

func (c *Client) GetTenantSecretForWorker(ctx context.Context, workerKey string, tenantSlug string, secretName string) (EncryptedTenantSecret, error) {
	var rows []EncryptedTenantSecret
	err := c.RPC(ctx, "get_tenant_secret_for_worker", map[string]any{
		"worker_key":  workerKey,
		"tenant_slug": tenantSlug,
		"secret_name": secretName,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	if err != nil {
		return EncryptedTenantSecret{}, err
	}
	if len(rows) == 0 {
		return EncryptedTenantSecret{}, fmt.Errorf("tenant secret not found: %s", secretName)
	}
	return rows[0], nil
}

func (c *Client) ListTenantSecrets(ctx context.Context, user auth.Principal, bearer string) ([]TenantSecret, error) {
	var rows []TenantSecret
	err := c.RPC(ctx, "list_tenant_secrets", map[string]any{}, user, bearer, &rows)
	return rows, err
}

func (c *Client) GetTenantSecret(ctx context.Context, user auth.Principal, bearer string, secretName string) (EncryptedTenantSecret, error) {
	var rows []EncryptedTenantSecret
	err := c.RPC(ctx, "get_tenant_secret", map[string]any{
		"secret_name": secretName,
	}, user, bearer, &rows)
	if err != nil {
		return EncryptedTenantSecret{}, err
	}
	if len(rows) == 0 {
		return EncryptedTenantSecret{}, fmt.Errorf("tenant secret not found: %s", secretName)
	}
	return rows[0], nil
}

func (c *Client) ListTenantSecretUsage(ctx context.Context, user auth.Principal, bearer string, secretName string) ([]TenantSecretUsage, error) {
	var rows []TenantSecretUsage
	err := c.RPC(ctx, "list_tenant_secret_usage", map[string]any{
		"secret_name": secretName,
	}, user, bearer, &rows)
	return rows, err
}

func (c *Client) SaveTenantSecret(ctx context.Context, user auth.Principal, bearer string, id string, name string, description string, ciphertext string, nonce string, keyVersion string) ([]TenantSecret, error) {
	var rows []TenantSecret
	var secretID any
	if strings.TrimSpace(id) != "" {
		secretID = id
	}
	err := c.RPC(ctx, "save_tenant_secret", map[string]any{
		"secret_id":          secretID,
		"secret_name":        name,
		"secret_description": description,
		"secret_ciphertext":  ciphertext,
		"secret_nonce":       nonce,
		"secret_key_version": keyVersion,
	}, user, bearer, &rows)
	return rows, err
}

func (c *Client) DeleteTenantSecret(ctx context.Context, user auth.Principal, bearer string, id string) ([]TenantSecret, error) {
	var rows []TenantSecret
	err := c.RPC(ctx, "delete_tenant_secret", map[string]any{
		"secret_id": id,
	}, user, bearer, &rows)
	return rows, err
}

func (c *Client) CurrentUserProfile(ctx context.Context, user auth.Principal, bearer string) (UserProfile, error) {
	var rows []UserProfile
	err := c.RPC(ctx, "current_user_profile", map[string]any{
		"user_subject":  user.Subject,
		"user_provider": user.Provider,
	}, user, bearer, &rows)
	if err != nil {
		return UserProfile{}, err
	}
	if len(rows) == 0 {
		return UserProfile{}, fmt.Errorf("current user profile not found")
	}
	return rows[0], nil
}

func (c *Client) CurrentUserHasRole(ctx context.Context, user auth.Principal, bearer string, allowed []string) (bool, error) {
	var ok bool
	err := c.RPC(ctx, "current_user_has_role", map[string]any{
		"user_subject":  user.Subject,
		"user_provider": user.Provider,
		"allowed_roles": allowed,
	}, user, bearer, &ok)
	return ok, err
}

func (c *Client) GetDataSource(ctx context.Context, user auth.Principal, bearer string, sourceID string) (DataSource, error) {
	var rows []DataSource
	err := c.RPC(ctx, "get_data_source", map[string]any{
		"source_id": sourceID,
	}, user, bearer, &rows)
	if err != nil {
		return DataSource{}, err
	}
	if len(rows) == 0 {
		return DataSource{}, fmt.Errorf("data source not found")
	}
	return rows[0], nil
}

func (c *Client) ListAlertIncidents(ctx context.Context, user auth.Principal, bearer string, limit int) ([]AlertIncident, error) {
	var rows []AlertIncident
	err := c.RPC(ctx, "list_alert_incidents", map[string]any{
		"incident_limit": limit,
	}, user, bearer, &rows)
	return rows, err
}

func (c *Client) ListAlertNotifications(ctx context.Context, user auth.Principal, bearer string, limit int) ([]AlertNotification, error) {
	var rows []AlertNotification
	err := c.RPC(ctx, "list_alert_notifications", map[string]any{
		"notification_limit": limit,
	}, user, bearer, &rows)
	return rows, err
}

func (c *Client) RecordContactTestNotification(ctx context.Context, user auth.Principal, bearer string, contactKind, contactTarget, status string, statusCode int, errorText string, payload map[string]any) (AlertNotification, error) {
	var rows []AlertNotification
	err := c.RPC(ctx, "record_contact_test_notification", map[string]any{
		"notification_contact_kind":   contactKind,
		"notification_contact_target": contactTarget,
		"delivery_status":             status,
		"delivery_status_code":        statusCode,
		"delivery_error":              errorText,
		"delivery_payload":            payload,
	}, user, bearer, &rows)
	if err != nil {
		return AlertNotification{}, err
	}
	if len(rows) == 0 {
		return AlertNotification{}, nil
	}
	return rows[0], nil
}

func (c *Client) RecordAlertIncident(ctx context.Context, workerKey, ruleID, tenantID, status string, value float64, payload map[string]any, fingerprint string, cooldownSeconds int) (AlertIncident, error) {
	var normalizedRuleID any
	if uuidPattern.MatchString(ruleID) {
		normalizedRuleID = ruleID
	}
	var rows []AlertIncident
	err := c.RPC(ctx, "record_alert_incident_for_worker", map[string]any{
		"worker_key":           workerKey,
		"rule_id":              normalizedRuleID,
		"tenant_slug":          tenantID,
		"incident_status":      status,
		"incident_value":       value,
		"incident_payload":     payload,
		"incident_fingerprint": fingerprint,
		"cooldown_seconds":     cooldownSeconds,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	if err != nil {
		return AlertIncident{}, err
	}
	if len(rows) == 0 {
		return AlertIncident{}, nil
	}
	return rows[0], nil
}

func (c *Client) UpdateAlertIncidentSync(ctx context.Context, workerKey, tenantID, incidentID, provider, externalID, externalURL, syncStatus, syncError string) (AlertIncident, error) {
	var normalizedIncidentID any
	if uuidPattern.MatchString(incidentID) {
		normalizedIncidentID = incidentID
	}
	var rows []AlertIncident
	err := c.RPC(ctx, "update_alert_incident_sync_for_worker", map[string]any{
		"worker_key":                 workerKey,
		"tenant_slug":                tenantID,
		"incident_id":                normalizedIncidentID,
		"sync_external_provider":     provider,
		"sync_external_incident_id":  externalID,
		"sync_external_incident_url": externalURL,
		"sync_status":                syncStatus,
		"sync_error":                 syncError,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	if err != nil {
		return AlertIncident{}, err
	}
	if len(rows) == 0 {
		return AlertIncident{}, nil
	}
	return rows[0], nil
}

func (c *Client) ListPagerDutySyncedIncidents(ctx context.Context, workerKey string) ([]PagerDutySyncedIncident, error) {
	var rows []PagerDutySyncedIncident
	err := c.RPC(ctx, "list_pagerduty_synced_incidents_for_worker", map[string]any{
		"worker_key": workerKey,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	return rows, err
}

func (c *Client) ReconcilePagerDutyIncident(ctx context.Context, workerKey, tenantID, incidentID, remoteStatus, externalURL, syncStatus, syncError string) (AlertIncident, error) {
	var normalizedIncidentID any
	if uuidPattern.MatchString(incidentID) {
		normalizedIncidentID = incidentID
	}
	var rows []AlertIncident
	err := c.RPC(ctx, "reconcile_pagerduty_incident_for_worker", map[string]any{
		"worker_key":                 workerKey,
		"tenant_slug":                tenantID,
		"incident_id":                normalizedIncidentID,
		"remote_status":              remoteStatus,
		"sync_external_incident_url": externalURL,
		"sync_status":                syncStatus,
		"sync_error":                 syncError,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	if err != nil {
		return AlertIncident{}, err
	}
	if len(rows) == 0 {
		return AlertIncident{}, nil
	}
	return rows[0], nil
}

func (c *Client) RecordAlertNotification(ctx context.Context, workerKey, ruleID, tenantID, incidentID, contactKind, contactTarget, status string, statusCode int, errorText string, payload map[string]any) (AlertNotification, error) {
	var normalizedRuleID any
	if uuidPattern.MatchString(ruleID) {
		normalizedRuleID = ruleID
	}
	var normalizedIncidentID any
	if uuidPattern.MatchString(incidentID) {
		normalizedIncidentID = incidentID
	}
	var rows []AlertNotification
	err := c.RPC(ctx, "record_alert_notification_for_worker", map[string]any{
		"worker_key":                  workerKey,
		"rule_id":                     normalizedRuleID,
		"tenant_slug":                 tenantID,
		"incident_id":                 normalizedIncidentID,
		"notification_contact_kind":   contactKind,
		"notification_contact_target": contactTarget,
		"delivery_status":             status,
		"delivery_status_code":        statusCode,
		"delivery_error":              errorText,
		"delivery_payload":            payload,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
	if err != nil {
		return AlertNotification{}, err
	}
	if len(rows) == 0 {
		return AlertNotification{}, nil
	}
	return rows[0], nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
