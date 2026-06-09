package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
