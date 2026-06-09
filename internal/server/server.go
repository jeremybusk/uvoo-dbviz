package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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
	a.mux.HandleFunc("GET /api/data-sources", a.requireAuth(a.listDataSources))
	a.mux.HandleFunc("POST /api/data-sources", a.requireAuth(a.saveDataSource))
	a.mux.HandleFunc("GET /api/dashboards", a.requireAuth(a.listDashboards))
	a.mux.HandleFunc("POST /api/dashboards", a.requireAuth(a.saveDashboard))
	a.mux.HandleFunc("GET /api/alerts/rules", a.requireAuth(a.listAlertRules))
	a.mux.HandleFunc("POST /api/alerts/rules", a.requireAuth(a.saveAlertRule))
	a.mux.HandleFunc("GET /api/alerts/contacts", a.requireAuth(a.listContactEndpoints))
	a.mux.HandleFunc("POST /api/alerts/contacts", a.requireAuth(a.saveContactEndpoint))
	a.mux.HandleFunc("GET /api/alerts/incidents", a.requireAuth(a.listAlertIncidents))
	a.mux.HandleFunc("POST /api/alerts/incidents/resolve", a.requireAuth(a.resolveAlertIncident))
	a.mux.HandleFunc("GET /api/members", a.requireAuth(a.listMembers))
	a.mux.HandleFunc("POST /api/members/role", a.requireAuth(a.updateMemberRole))
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
	rows, err := a.ch.QueryJSONEachRow(r.Context(), sql)
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
	writeJSON(w, http.StatusOK, rows)
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
	queryBytes, err := json.Marshal(req.Query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var alertQuery clickhouse.QueryRequest
	if err := json.Unmarshal(queryBytes, &alertQuery); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ds, ok := a.cfg.Datasets[alertQuery.Dataset]
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("unknown alert dataset"))
		return
	}
	if _, err := clickhouse.BuildTimeseriesSQL(alertQuery, ds, statePrincipal(r).TenantID, a.cfg.ClickHouse.MaxRows); err != nil {
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
	err = a.state.RPC(r.Context(), "save_alert_rule", map[string]any{
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
	writeJSON(w, http.StatusOK, rows)
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
