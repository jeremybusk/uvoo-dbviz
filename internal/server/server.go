package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	a.mux.HandleFunc("POST /api/query", a.requireAuth(a.query))
	a.mux.HandleFunc("GET /api/dashboards", a.requireAuth(a.listDashboards))
	a.mux.HandleFunc("POST /api/dashboards", a.requireAuth(a.saveDashboard))
	a.mux.HandleFunc("GET /api/alerts/rules", a.requireAuth(a.listAlertRules))
	a.mux.HandleFunc("POST /api/alerts/rules", a.requireAuth(a.saveAlertRule))
	a.mux.HandleFunc("GET /api/alerts/contacts", a.requireAuth(a.listContactEndpoints))
	a.mux.HandleFunc("POST /api/alerts/contacts", a.requireAuth(a.saveContactEndpoint))
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

func (a *App) query(w http.ResponseWriter, r *http.Request) {
	var req clickhouse.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ds, ok := a.cfg.Datasets[req.Dataset]
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("unknown dataset"))
		return
	}
	sql, err := clickhouse.BuildTimeseriesSQL(req, ds, principal(r).TenantID, a.cfg.ClickHouse.MaxRows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := a.ch.QueryJSONEachRow(r.Context(), sql)
	if err != nil {
		a.logger.Warn("clickhouse query failed", "error", err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (a *App) listDashboards(w http.ResponseWriter, r *http.Request) {
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_dashboards", map[string]any{}, principal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveDashboard(w http.ResponseWriter, r *http.Request) {
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
	}, principal(r), r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) listAlertRules(w http.ResponseWriter, r *http.Request) {
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "list_alert_rules", map[string]any{}, principal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveAlertRule(w http.ResponseWriter, r *http.Request) {
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
	if _, err := clickhouse.BuildTimeseriesSQL(alertQuery, ds, principal(r).TenantID, a.cfg.ClickHouse.MaxRows); err != nil {
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
	}, principal(r), r.Header.Get("Authorization"), &rows)
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
	if err := a.state.RPC(r.Context(), "list_contact_endpoints", map[string]any{}, principal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveContactEndpoint(w http.ResponseWriter, r *http.Request) {
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
	}, principal(r), r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.authn.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
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
