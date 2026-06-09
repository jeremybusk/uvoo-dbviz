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
	baseURL string
	http    *http.Client
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

type AlertIncident struct {
	ID          string         `json:"id"`
	AlertRuleID string         `json:"alert_rule_id"`
	Status      string         `json:"status"`
	Value       float64        `json:"value"`
	Payload     map[string]any `json:"payload"`
	CreatedAt   string         `json:"created_at"`
}

func NewClient(cfg config.PostgRESTConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(cfg.URL, "/"), http: httpClient}
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
	if bearer != "" {
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

func (c *Client) ListAlertIncidents(ctx context.Context, user auth.Principal, bearer string, limit int) ([]AlertIncident, error) {
	var rows []AlertIncident
	err := c.RPC(ctx, "list_alert_incidents", map[string]any{
		"incident_limit": limit,
	}, user, bearer, &rows)
	return rows, err
}

func (c *Client) RecordAlertIncident(ctx context.Context, workerKey, ruleID, tenantID, status string, value float64, payload map[string]any) error {
	var normalizedRuleID any
	if uuidPattern.MatchString(ruleID) {
		normalizedRuleID = ruleID
	}
	var rows []AlertIncident
	return c.RPC(ctx, "record_alert_incident_for_worker", map[string]any{
		"worker_key":       workerKey,
		"rule_id":          normalizedRuleID,
		"tenant_slug":      tenantID,
		"incident_status":  status,
		"incident_value":   value,
		"incident_payload": payload,
	}, auth.Principal{TenantID: "dev", Email: "worker@localhost"}, "", &rows)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
