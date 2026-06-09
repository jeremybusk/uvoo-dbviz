package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"uvoo-dbviz/internal/auth"
	"uvoo-dbviz/internal/clickhouse"
	"uvoo-dbviz/internal/config"
	"uvoo-dbviz/internal/state"
)

type App struct {
	cfg    config.Config
	authn  *auth.Manager
	ch     *clickhouse.Client
	state  *state.Client
	logger *slog.Logger
	mux    *http.ServeMux
}

func New(cfg config.Config, authn *auth.Manager, ch *clickhouse.Client, stateClient *state.Client, logger *slog.Logger) http.Handler {
	app := &App{cfg: cfg, authn: authn, ch: ch, state: stateClient, logger: logger, mux: http.NewServeMux()}
	app.routes()
	return securityHeaders(app.mux)
}

func (a *App) routes() {
	a.mux.HandleFunc("GET /healthz", a.health)
	a.mux.HandleFunc("GET /api/config", a.publicConfig)
	a.mux.HandleFunc("GET /api/oidc/{provider}/discovery", a.oidcDiscovery)
	a.mux.HandleFunc("POST /api/oidc/{provider}/exchange", a.oidcExchange)
	a.mux.HandleFunc("GET /api/me", a.requireAuth(a.me))
	a.mux.HandleFunc("POST /api/session/sync", a.requireAuth(a.syncSession))
	a.mux.HandleFunc("GET /api/session/profile", a.requireAuth(a.sessionProfile))
	a.mux.HandleFunc("GET /api/session/memberships", a.requireAuth(a.listMemberships))
	a.mux.HandleFunc("POST /api/query", a.requireAuth(a.query))
	a.mux.HandleFunc("GET /api/query/history", a.requireAuth(a.listQueryHistory))
	a.mux.HandleFunc("GET /api/saved-queries", a.requireAuth(a.listSavedQueries))
	a.mux.HandleFunc("POST /api/saved-queries", a.requireAuth(a.saveSavedQuery))
	a.mux.HandleFunc("GET /api/audit/events", a.requireAuth(a.listAuditEvents))
	a.mux.HandleFunc("GET /api/data-sources", a.requireAuth(a.listDataSources))
	a.mux.HandleFunc("POST /api/data-sources", a.requireAuth(a.saveDataSource))
	a.mux.HandleFunc("POST /api/data-sources/test", a.requireAuth(a.testDataSource))
	a.mux.HandleFunc("GET /api/dashboards", a.requireAuth(a.listDashboards))
	a.mux.HandleFunc("POST /api/dashboards", a.requireAuth(a.saveDashboard))
	a.mux.HandleFunc("GET /api/alerts/rules", a.requireAuth(a.listAlertRules))
	a.mux.HandleFunc("POST /api/alerts/rules", a.requireAuth(a.saveAlertRule))
	a.mux.HandleFunc("GET /api/alerts/contacts", a.requireAuth(a.listContactEndpoints))
	a.mux.HandleFunc("POST /api/alerts/contacts", a.requireAuth(a.saveContactEndpoint))
	a.mux.HandleFunc("GET /api/alerts/incidents", a.requireAuth(a.listAlertIncidents))
	a.mux.HandleFunc("GET /api/alerts/notifications", a.requireAuth(a.listAlertNotifications))
	a.mux.HandleFunc("POST /api/alerts/incidents/resolve", a.requireAuth(a.resolveAlertIncident))
	a.mux.HandleFunc("GET /api/members", a.requireAuth(a.listMembers))
	a.mux.HandleFunc("POST /api/members/role", a.requireAuth(a.updateMemberRole))
	a.mux.HandleFunc("POST /api/members/deactivate", a.requireAuth(a.deactivateMember))
	a.mux.HandleFunc("GET /api/invites", a.requireAuth(a.listInvites))
	a.mux.HandleFunc("POST /api/invites", a.requireAuth(a.createInvite))
	a.mux.HandleFunc("POST /api/invites/accept", a.requireAuth(a.acceptInvite))
	a.mux.HandleFunc("/", a.static)
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) publicConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.cfg.Public())
}

func (a *App) oidcDiscovery(w http.ResponseWriter, r *http.Request) {
	result, err := a.authn.PublicDiscovery(r.Context(), r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) oidcExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code         string `json:"code"`
		RedirectURI  string `json:"redirectUri"`
		CodeVerifier string `json:"codeVerifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tokens, err := a.authn.ExchangeCode(r.Context(), r.PathValue("provider"), req.Code, req.RedirectURI, req.CodeVerifier)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, principal(r))
}

func (a *App) syncSession(w http.ResponseWriter, r *http.Request) {
	user := principal(r)
	var rows []map[string]any
	err := a.state.RPC(r.Context(), "sync_current_user", map[string]any{
		"user_subject":  user.Subject,
		"user_email":    user.Email,
		"user_name":     user.Name,
		"user_provider": user.Provider,
		"tenant_slug":   user.TenantID,
		"tenant_name":   user.TenantID,
	}, user, r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) sessionProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := a.state.CurrentUserProfile(r.Context(), statePrincipal(r), r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (a *App) listMemberships(w http.ResponseWriter, r *http.Request) {
	user := principal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_user_memberships", map[string]any{
		"user_subject":  user.Subject,
		"user_provider": user.Provider,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) query(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor", "viewer") {
		return
	}
	start := time.Now()
	var req clickhouse.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ds, ok := a.cfg.Datasets[req.Dataset]
	if !ok {
		a.recordQueryHistory(r, req, 0, start, "failed", "unknown dataset")
		writeError(w, http.StatusBadRequest, errors.New("unknown dataset"))
		return
	}
	sql, err := clickhouse.BuildTimeseriesSQL(req, ds, statePrincipal(r).TenantID, a.cfg.ClickHouse.MaxRows)
	if err != nil {
		a.recordQueryHistory(r, req, 0, start, "failed", err.Error())
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ch, err := a.clickHouseForQuery(r, req)
	if err != nil {
		a.recordQueryHistory(r, req, 0, start, "failed", err.Error())
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := ch.QueryJSONEachRow(r.Context(), sql)
	if err != nil {
		a.logger.Warn("clickhouse query failed", "error", err)
		a.recordQueryHistory(r, req, 0, start, "failed", err.Error())
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordQueryHistory(r, req, len(rows), start, "success", "")
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (a *App) listQueryHistory(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor", "viewer") {
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_query_history", map[string]any{
		"history_limit": 50,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) listSavedQueries(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor", "viewer") {
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_saved_queries", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveSavedQuery(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID          string         `json:"id"`
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Query       map[string]any `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("saved query name is required"))
		return
	}
	if err := a.validateQueryPayload(req.Query, statePrincipal(r).TenantID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var savedQueryID any
	if req.ID != "" {
		savedQueryID = req.ID
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "save_saved_query", map[string]any{
		"saved_query_id":          savedQueryID,
		"saved_query_name":        req.Name,
		"saved_query_description": req.Description,
		"saved_query_payload":     req.Query,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "saved_query.save", "saved_query", targetIDFromRows(rows), map[string]any{"name": req.Name})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) recordQueryHistory(r *http.Request, req clickhouse.QueryRequest, rowsCount int, start time.Time, status string, errText string) {
	user := statePrincipal(r)
	if err := a.state.RPC(r.Context(), "record_query_history", map[string]any{
		"user_subject":      user.Subject,
		"user_provider":     user.Provider,
		"query_dataset":     req.Dataset,
		"query_payload":     req,
		"query_rows_count":  rowsCount,
		"query_duration_ms": int(time.Since(start).Milliseconds()),
		"query_status":      status,
		"query_error":       errText,
	}, user, r.Header.Get("Authorization"), nil); err != nil {
		a.logger.Warn("query history record failed", "error", err)
	}
}

func (a *App) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_audit_events", map[string]any{
		"event_limit": 100,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) recordAuditEvent(r *http.Request, action, targetType, targetID string, payload map[string]any) {
	user := statePrincipal(r)
	if payload == nil {
		payload = map[string]any{}
	}
	if err := a.state.RPC(r.Context(), "record_audit_event", map[string]any{
		"actor_subject":     user.Subject,
		"actor_provider":    user.Provider,
		"actor_email":       user.Email,
		"event_action":      action,
		"event_target_type": targetType,
		"event_target_id":   targetID,
		"event_payload":     payload,
	}, user, r.Header.Get("Authorization"), nil); err != nil {
		a.logger.Warn("audit event record failed", "action", action, "error", err)
	}
}

func (a *App) listDataSources(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor", "viewer") {
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_data_sources", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) testDataSource(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("data source id is required"))
		return
	}
	source, err := a.state.GetDataSource(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), req.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	cfg, err := a.clickHouseConfigFromSource(source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	start := time.Now()
	client := clickhouse.NewClient(cfg, nil)
	rows, err := client.QueryJSONEachRow(r.Context(), "SELECT 1 AS ok FORMAT JSONEachRow")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"rows":       rows,
		"durationMs": int(time.Since(start).Milliseconds()),
	})
}

func (a *App) saveDataSource(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var req struct {
		ID     string         `json:"id"`
		Name   string         `json:"name"`
		Kind   string         `json:"kind"`
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Name == "" || req.Kind == "" {
		writeError(w, http.StatusBadRequest, errors.New("data source name and kind are required"))
		return
	}
	if req.Kind != "clickhouse" {
		writeError(w, http.StatusBadRequest, errors.New("only clickhouse data sources are supported"))
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	if _, hasPassword := req.Config["password"]; hasPassword {
		writeError(w, http.StatusBadRequest, errors.New("store data source passwords in a secret manager and pass passwordSecretRef"))
		return
	}
	if urlValue, _ := req.Config["url"].(string); strings.TrimSpace(urlValue) == "" {
		writeError(w, http.StatusBadRequest, errors.New("data source config.url is required"))
		return
	} else if err := validateDataSourceURL(urlValue); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var sourceID any
	if req.ID != "" {
		sourceID = req.ID
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "save_data_source", map[string]any{
		"source_id":     sourceID,
		"source_name":   req.Name,
		"source_kind":   req.Kind,
		"source_config": req.Config,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "data_source.save", "data_source", targetIDFromRows(rows), map[string]any{"name": req.Name, "kind": req.Kind})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) clickHouseForQuery(r *http.Request, req clickhouse.QueryRequest) (*clickhouse.Client, error) {
	if strings.TrimSpace(req.SourceID) == "" {
		return a.ch, nil
	}
	source, err := a.state.GetDataSource(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), req.SourceID)
	if err != nil {
		return nil, err
	}
	cfg, err := a.clickHouseConfigFromSource(source)
	if err != nil {
		return nil, err
	}
	return clickhouse.NewClient(cfg, nil), nil
}

func (a *App) clickHouseConfigFromSource(source state.DataSource) (config.ClickHouseConfig, error) {
	if source.Kind != "clickhouse" {
		return config.ClickHouseConfig{}, errors.New("data source is not clickhouse")
	}
	cfg := a.cfg.ClickHouse
	if value, _ := source.Config["url"].(string); strings.TrimSpace(value) != "" {
		if err := validateDataSourceURL(value); err != nil {
			return config.ClickHouseConfig{}, err
		}
		cfg.URL = strings.TrimSpace(value)
	}
	if value, _ := source.Config["database"].(string); strings.TrimSpace(value) != "" {
		cfg.Database = strings.TrimSpace(value)
	}
	if value, _ := source.Config["username"].(string); strings.TrimSpace(value) != "" {
		cfg.User = strings.TrimSpace(value)
	}
	if value, _ := source.Config["passwordSecretRef"].(string); strings.TrimSpace(value) != "" {
		secretValue, ok := resolveSecretRef(value)
		if !ok {
			return config.ClickHouseConfig{}, fmt.Errorf("secret ref is not configured: %s", value)
		}
		cfg.Password = secretValue
	}
	return cfg, nil
}

func validateDataSourceURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("data source url must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("data source url host is required")
	}
	if parsed.User != nil {
		return errors.New("data source url must not include credentials")
	}
	return nil
}

func resolveSecretRef(ref string) (string, bool) {
	name := secretEnvName(ref)
	value, ok := os.LookupEnv(name)
	if ok {
		return value, true
	}
	return "", false
}

var secretRefCleaner = regexp.MustCompile(`[^A-Za-z0-9]+`)

func secretEnvName(ref string) string {
	clean := strings.Trim(secretRefCleaner.ReplaceAllString(strings.ToUpper(strings.TrimSpace(ref)), "_"), "_")
	return "DBVIZ_SECRET_" + clean
}

func (a *App) listDashboards(w http.ResponseWriter, r *http.Request) {
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_dashboards", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID     string         `json:"id"`
		Name   string         `json:"name"`
		Layout map[string]any `json:"layout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("dashboard name is required"))
		return
	}
	var dashboardID any
	if req.ID != "" {
		dashboardID = req.ID
	}
	var rows []map[string]any
	err := a.state.RPC(r.Context(), "save_dashboard", map[string]any{
		"dashboard_id":     dashboardID,
		"dashboard_name":   req.Name,
		"dashboard_layout": req.Layout,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "dashboard.save", "dashboard", targetIDFromRows(rows), map[string]any{"name": req.Name})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) listAlertRules(w http.ResponseWriter, r *http.Request) {
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_alert_rules", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveAlertRule(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID                string         `json:"id"`
		Name              string         `json:"name"`
		Query             map[string]any `json:"query"`
		Condition         map[string]any `json:"condition"`
		IntervalSeconds   int            `json:"intervalSeconds"`
		Enabled           bool           `json:"enabled"`
		ContactEndpointID string         `json:"contactEndpointId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("alert name is required"))
		return
	}
	if err := a.validateQueryPayload(req.Query, statePrincipal(r).TenantID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Condition == nil {
		writeError(w, http.StatusBadRequest, errors.New("alert condition is required"))
		return
	}
	threshold, ok := numericValue(req.Condition["threshold"])
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("alert threshold must be numeric"))
		return
	}
	req.Condition["threshold"] = threshold
	var alertID, contactID any
	if req.ID != "" {
		alertID = req.ID
	}
	if req.ContactEndpointID != "" {
		contactID = req.ContactEndpointID
	}
	var rows []map[string]any
	err := a.state.RPC(r.Context(), "save_alert_rule", map[string]any{
		"alert_id":         alertID,
		"alert_name":       req.Name,
		"alert_query":      req.Query,
		"alert_condition":  req.Condition,
		"alert_interval":   req.IntervalSeconds,
		"alert_enabled":    req.Enabled,
		"alert_contact_id": contactID,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "alert_rule.save", "alert_rule", targetIDFromRows(rows), map[string]any{"name": req.Name, "enabled": req.Enabled})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) validateQueryPayload(payload map[string]any, tenantID string) error {
	if payload == nil {
		return errors.New("query payload is required")
	}
	queryBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var query clickhouse.QueryRequest
	if err := json.Unmarshal(queryBytes, &query); err != nil {
		return err
	}
	ds, ok := a.cfg.Datasets[query.Dataset]
	if !ok {
		return errors.New("unknown query dataset")
	}
	_, err = clickhouse.BuildTimeseriesSQL(query, ds, tenantID, a.cfg.ClickHouse.MaxRows)
	return err
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (a *App) listContactEndpoints(w http.ResponseWriter, r *http.Request) {
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_contact_endpoints", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveContactEndpoint(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID     string         `json:"id"`
		Name   string         `json:"name"`
		Kind   string         `json:"kind"`
		Target string         `json:"target"`
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" || req.Kind == "" || req.Target == "" {
		writeError(w, http.StatusBadRequest, errors.New("contact name, kind, and target are required"))
		return
	}
	var contactID any
	if req.ID != "" {
		contactID = req.ID
	}
	var rows []map[string]any
	err := a.state.RPC(r.Context(), "save_contact_endpoint", map[string]any{
		"contact_id":     contactID,
		"contact_name":   req.Name,
		"contact_kind":   req.Kind,
		"contact_target": req.Target,
		"contact_config": req.Config,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "contact_endpoint.save", "contact_endpoint", targetIDFromRows(rows), map[string]any{"name": req.Name, "kind": req.Kind})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) listAlertIncidents(w http.ResponseWriter, r *http.Request) {
	rows, err := a.state.ListAlertIncidents(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), 100)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) listAlertNotifications(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor", "viewer") {
		return
	}
	rows, err := a.state.ListAlertNotifications(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), 100)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) resolveAlertIncident(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("incident id is required"))
		return
	}
	user := statePrincipal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "resolve_alert_incident", map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
		"incident_id":    req.ID,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "alert_incident.resolve", "alert_incident", req.ID, nil)
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) listInvites(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_tenant_invites", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) createInvite(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, errors.New("invite email is required"))
		return
	}
	if req.Role == "" {
		req.Role = "viewer"
	}
	user := statePrincipal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "create_tenant_invite", map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
		"invite_email":   req.Email,
		"invite_role":    req.Role,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "tenant_invite.create", "tenant_invite", targetIDFromRows(rows), map[string]any{"email": req.Email, "role": req.Role})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, errors.New("invite token is required"))
		return
	}
	user := principal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "accept_tenant_invite", map[string]any{
		"invite_token":  req.Token,
		"user_subject":  user.Subject,
		"user_email":    user.Email,
		"user_name":     user.Name,
		"user_provider": user.Provider,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) deactivateMember(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("member id is required"))
		return
	}
	user := statePrincipal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "deactivate_tenant_member", map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
		"member_id":      req.ID,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "member.deactivate", "user", req.ID, nil)
	writeJSON(w, http.StatusOK, rows)
}

func targetIDFromRows(rows []map[string]any) string {
	if len(rows) == 0 {
		return ""
	}
	if value, ok := rows[0]["id"].(string); ok {
		return value
	}
	return fmt.Sprint(rows[0]["id"])
}

func (a *App) listMembers(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	user := statePrincipal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_tenant_members", map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin") {
		return
	}
	var req struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Role = strings.TrimSpace(req.Role)
	if req.ID == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, errors.New("member id and role are required"))
		return
	}
	user := statePrincipal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "update_tenant_member_role", map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
		"member_id":      req.ID,
		"member_role":    req.Role,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "member.role_update", "user", req.ID, map[string]any{"role": req.Role})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) requireStateRole(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	ok, err := a.state.CurrentUserHasRole(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), allowed)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, errors.New("insufficient role"))
		return false
	}
	return true
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.authn.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		if activeTenant := strings.TrimSpace(r.Header.Get("X-DBViz-Tenant")); activeTenant != "" {
			if user.Headers == nil {
				user.Headers = map[string]string{}
			}
			user.Headers["ActiveTenantID"] = activeTenant
		}
		ctx := r.Context()
		ctx = withPrincipal(ctx, user)
		next(w, r.WithContext(ctx))
	}
}

func (a *App) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	full := filepath.Join(a.cfg.WebRoot, filepath.Clean(path))
	if !strings.HasPrefix(full, filepath.Clean(a.cfg.WebRoot)) {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(full); err != nil {
		full = filepath.Join(a.cfg.WebRoot, "index.html")
	}
	http.ServeFile(w, r, full)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
