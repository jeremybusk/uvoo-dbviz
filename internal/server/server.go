package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"uvoo-sqviz/internal/alert"
	"uvoo-sqviz/internal/auth"
	"uvoo-sqviz/internal/clickhouse"
	"uvoo-sqviz/internal/config"
	"uvoo-sqviz/internal/secrets"
	"uvoo-sqviz/internal/state"
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
	a.mux.HandleFunc("GET /api/session/preferences", a.requireAuth(a.sessionPreferences))
	a.mux.HandleFunc("POST /api/session/preferences", a.requireAuth(a.saveSessionPreferences))
	a.mux.HandleFunc("GET /api/session/memberships", a.requireAuth(a.listMemberships))
	a.mux.HandleFunc("GET /api/system/readiness", a.requireAuth(a.systemReadiness))
	a.mux.HandleFunc("POST /api/query", a.requireAuth(a.query))
	a.mux.HandleFunc("POST /api/events", a.requireAuth(a.events))
	a.mux.HandleFunc("POST /api/sql", a.requireAuth(a.customSQL))
	a.mux.HandleFunc("GET /api/query/history", a.requireAuth(a.listQueryHistory))
	a.mux.HandleFunc("GET /api/saved-queries", a.requireAuth(a.listSavedQueries))
	a.mux.HandleFunc("POST /api/saved-queries", a.requireAuth(a.saveSavedQuery))
	a.mux.HandleFunc("POST /api/saved-queries/delete", a.requireAuth(a.deleteSavedQuery))
	a.mux.HandleFunc("GET /api/audit/events", a.requireAuth(a.listAuditEvents))
	a.mux.HandleFunc("GET /api/data-sources", a.requireAuth(a.listDataSources))
	a.mux.HandleFunc("POST /api/data-sources", a.requireAuth(a.saveDataSource))
	a.mux.HandleFunc("POST /api/data-sources/delete", a.requireAuth(a.deleteDataSource))
	a.mux.HandleFunc("POST /api/data-sources/test", a.requireAuth(a.testDataSource))
	a.mux.HandleFunc("GET /api/dashboards", a.requireAuth(a.listDashboards))
	a.mux.HandleFunc("POST /api/dashboards", a.requireAuth(a.saveDashboard))
	a.mux.HandleFunc("POST /api/dashboards/delete", a.requireAuth(a.deleteDashboard))
	a.mux.HandleFunc("GET /api/alerts/rules", a.requireAuth(a.listAlertRules))
	a.mux.HandleFunc("POST /api/alerts/rules", a.requireAuth(a.saveAlertRule))
	a.mux.HandleFunc("POST /api/alerts/rules/delete", a.requireAuth(a.deleteAlertRule))
	a.mux.HandleFunc("POST /api/alerts/test", a.requireAuth(a.testAlertRule))
	a.mux.HandleFunc("GET /api/alerts/contacts", a.requireAuth(a.listContactEndpoints))
	a.mux.HandleFunc("POST /api/alerts/contacts", a.requireAuth(a.saveContactEndpoint))
	a.mux.HandleFunc("POST /api/alerts/contacts/delete", a.requireAuth(a.deleteContactEndpoint))
	a.mux.HandleFunc("POST /api/alerts/contacts/test", a.requireAuth(a.testContactEndpoint))
	a.mux.HandleFunc("GET /api/secrets", a.requireAuth(a.listTenantSecrets))
	a.mux.HandleFunc("POST /api/secrets", a.requireAuth(a.saveTenantSecret))
	a.mux.HandleFunc("POST /api/secrets/delete", a.requireAuth(a.deleteTenantSecret))
	a.mux.HandleFunc("GET /api/alerts/incidents", a.requireAuth(a.listAlertIncidents))
	a.mux.HandleFunc("GET /api/alerts/notifications", a.requireAuth(a.listAlertNotifications))
	a.mux.HandleFunc("POST /api/alerts/incidents/acknowledge", a.requireAuth(a.acknowledgeAlertIncident))
	a.mux.HandleFunc("POST /api/alerts/incidents/resolve", a.requireAuth(a.resolveAlertIncident))
	a.mux.HandleFunc("POST /api/alerts/pagerduty/sync", a.requireAuth(a.syncPagerDutyIncidents))
	a.mux.HandleFunc("GET /api/members", a.requireAuth(a.listMembers))
	a.mux.HandleFunc("POST /api/members/role", a.requireAuth(a.updateMemberRole))
	a.mux.HandleFunc("POST /api/members/deactivate", a.requireAuth(a.deactivateMember))
	a.mux.HandleFunc("GET /api/invites", a.requireAuth(a.listInvites))
	a.mux.HandleFunc("POST /api/invites", a.requireAuth(a.createInvite))
	a.mux.HandleFunc("POST /api/invites/delete", a.requireAuth(a.deleteInvite))
	a.mux.HandleFunc("POST /api/invites/accept", a.requireAuth(a.acceptInvite))
	a.mux.HandleFunc("/", a.static)
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) publicConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.cfg.Public())
}

func (a *App) systemReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	components := []map[string]any{}
	overall := "ok"
	add := func(name string, status string, detail string) {
		if status == "failed" {
			overall = "failed"
		} else if status == "warning" && overall == "ok" {
			overall = "warning"
		}
		components = append(components, map[string]any{
			"name":   name,
			"status": status,
			"detail": detail,
		})
	}
	if err := a.ch.Ping(ctx); err != nil {
		add("ClickHouse", "failed", err.Error())
	} else {
		add("ClickHouse", "ok", "query endpoint reachable")
	}
	if err := a.state.Ping(ctx); err != nil {
		add("Postgres/PostgREST", "failed", err.Error())
	} else {
		add("Postgres/PostgREST", "ok", "state API reachable")
	}
	if a.cfg.Alerts.Enabled {
		detail := "alert worker enabled"
		if a.cfg.Alerts.LoadPersisted {
			detail += ", persisted rules enabled"
		}
		add("Alert worker", "ok", detail)
	} else {
		add("Alert worker", "warning", "SQVIZ_ALERTS_ENABLED is false")
	}
	if strings.TrimSpace(a.cfg.Alerts.SMTPHost) != "" && strings.TrimSpace(a.cfg.Alerts.SMTPFrom) != "" {
		detail := "SMTP host and sender configured"
		if strings.TrimSpace(a.cfg.Alerts.SMTPUser) != "" {
			detail += ", auth configured"
		}
		add("SMTP", "ok", detail)
	} else {
		add("SMTP", "warning", "email contacts will be skipped until SMTP host and sender are set")
	}
	if strings.TrimSpace(a.cfg.Secrets.EncryptionKey) != "" {
		add("Secrets", "ok", "tenant secret encryption key configured")
	} else {
		add("Secrets", "failed", "SQVIZ_SECRETS_ENCRYPTION_KEY is required for stored tenant secrets")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     overall,
		"checkedAt":  time.Now().UTC().Format(time.RFC3339),
		"components": components,
	})
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
	rows, err := a.syncCurrentUser(r, user)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) syncCurrentUser(r *http.Request, user auth.Principal) ([]map[string]any, error) {
	var rows []map[string]any
	err := a.state.RPC(r.Context(), "sync_current_user", map[string]any{
		"user_subject":  user.Subject,
		"user_email":    user.Email,
		"user_name":     user.Name,
		"user_provider": user.Provider,
		"tenant_slug":   user.TenantID,
		"tenant_name":   user.TenantID,
	}, user, r.Header.Get("Authorization"), &rows)
	return rows, err
}

func (a *App) sessionProfile(w http.ResponseWriter, r *http.Request) {
	if _, err := a.syncCurrentUser(r, principal(r)); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	profile, err := a.state.CurrentUserProfile(r.Context(), statePrincipal(r), r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (a *App) sessionPreferences(w http.ResponseWriter, r *http.Request) {
	if _, err := a.syncCurrentUser(r, principal(r)); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	user := statePrincipal(r)
	var preferences map[string]any
	if err := a.state.RPC(r.Context(), "current_user_preferences", map[string]any{
		"user_subject":  user.Subject,
		"user_provider": user.Provider,
	}, user, r.Header.Get("Authorization"), &preferences); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if preferences == nil {
		preferences = map[string]any{}
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (a *App) saveSessionPreferences(w http.ResponseWriter, r *http.Request) {
	if _, err := a.syncCurrentUser(r, principal(r)); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user := statePrincipal(r)
	var preferences map[string]any
	if err := a.state.RPC(r.Context(), "save_current_user_preferences", map[string]any{
		"user_subject":     user.Subject,
		"user_provider":    user.Provider,
		"user_preferences": sanitizePreferences(req),
	}, user, r.Header.Get("Authorization"), &preferences); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if preferences == nil {
		preferences = map[string]any{}
	}
	writeJSON(w, http.StatusOK, preferences)
}

func sanitizePreferences(input map[string]any) map[string]any {
	out := map[string]any{}
	if value, _ := input["themeMode"].(string); value == "light" || value == "dark" {
		out["themeMode"] = value
	}
	if value, ok := numericValue(input["refreshSeconds"]); ok {
		seconds := int(value)
		switch seconds {
		case 0, 10, 30, 60, 300:
			out["refreshSeconds"] = seconds
		}
	}
	if value, ok := numericValue(input["eventLimit"]); ok {
		limit := int(value)
		if limit >= 10 && limit <= 1000 {
			out["eventLimit"] = limit
		}
	}
	if value, _ := input["dataset"].(string); value != "" && len(value) <= 80 {
		out["dataset"] = strings.TrimSpace(value)
	}
	if value, _ := input["sourceId"].(string); len(value) <= 80 {
		out["sourceId"] = strings.TrimSpace(value)
	}
	if value, _ := input["visualization"].(string); value == "line" || value == "area" || value == "bar" {
		out["visualization"] = value
	}
	if rawRange, ok := input["relativeRange"].(map[string]any); ok {
		if value, valueOK := numericValue(rawRange["value"]); valueOK {
			if unit, unitOK := rawRange["unit"].(string); unitOK && validPreferenceRangeUnit(unit) {
				rangeValue := int(value)
				if rangeValue >= 1 && rangeValue <= 999 {
					out["relativeRange"] = map[string]any{"value": rangeValue, "unit": unit}
				}
			}
		}
	}
	return out
}

func validPreferenceRangeUnit(unit string) bool {
	switch unit {
	case "minutes", "hours", "days", "weeks", "months", "years":
		return true
	default:
		return false
	}
}

func (a *App) listMemberships(w http.ResponseWriter, r *http.Request) {
	user := principal(r)
	if _, err := a.syncCurrentUser(r, user); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
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

func (a *App) events(w http.ResponseWriter, r *http.Request) {
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
	sql, err := clickhouse.BuildEventsSQL(req, ds, statePrincipal(r).TenantID, a.cfg.ClickHouse.MaxRows)
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
		a.logger.Warn("clickhouse events query failed", "error", err)
		a.recordQueryHistory(r, req, 0, start, "failed", err.Error())
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordQueryHistory(r, req, len(rows), start, "success", "")
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (a *App) customSQL(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor", "viewer") {
		return
	}
	start := time.Now()
	var req clickhouse.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := a.runCustomSQL(r, req, clickhouse.CustomSQLExplore)
	if err != nil {
		a.recordQueryHistory(r, req, 0, start, "failed", err.Error())
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.recordQueryHistory(r, req, len(rows), start, "success", "")
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (a *App) runCustomSQL(r *http.Request, req clickhouse.QueryRequest, mode clickhouse.CustomSQLMode) ([]map[string]any, error) {
	ds, ok := a.cfg.Datasets[req.Dataset]
	if !ok {
		return nil, errors.New("unknown dataset")
	}
	built, err := clickhouse.BuildCustomSQL(req, ds, statePrincipal(r).TenantID, a.cfg.ClickHouse.MaxRows, mode)
	if err != nil {
		return nil, err
	}
	ch, err := a.clickHouseForQuery(r, req)
	if err != nil {
		return nil, err
	}
	rows, err := ch.QueryJSONEachRowWithParams(r.Context(), built.SQL, built.Params)
	if err != nil {
		a.logger.Warn("clickhouse custom sql failed", "error", err)
		return nil, err
	}
	return rows, nil
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
	if err := a.validateQueryPayload(req.Query, statePrincipal(r).TenantID, clickhouse.CustomSQLExplore); err != nil {
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

func (a *App) deleteDataSource(w http.ResponseWriter, r *http.Request) {
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
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "delete_data_source", map[string]any{
		"source_id": req.ID,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "data_source.delete", "data_source", req.ID, nil)
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
	name := alert.SecretEnvName(ref)
	value, ok := os.LookupEnv(name)
	if ok {
		return value, true
	}
	return "", false
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
	req.Name = strings.TrimSpace(req.Name)
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

func (a *App) deleteSavedQuery(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, errors.New("saved query id is required"))
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "delete_saved_query", map[string]any{
		"saved_query_id": req.ID,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "saved_query.delete", "saved_query", req.ID, nil)
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) deleteDashboard(w http.ResponseWriter, r *http.Request) {
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
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("dashboard id is required"))
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "delete_dashboard", map[string]any{
		"dashboard_id": req.ID,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "dashboard.delete", "dashboard", req.ID, nil)
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
	if err := a.validateQueryPayload(req.Query, statePrincipal(r).TenantID, clickhouse.CustomSQLAlert); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Condition == nil {
		writeError(w, http.StatusBadRequest, errors.New("alert condition is required"))
		return
	}
	condition, err := normalizeAlertCondition(req.Condition)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	applyNormalizedAlertCondition(req.Condition, condition)
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
	a.recordAuditEvent(r, "alert_rule.save", "alert_rule", targetIDFromRows(rows), map[string]any{"name": req.Name, "enabled": req.Enabled})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, errors.New("alert rule id is required"))
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "delete_alert_rule", map[string]any{
		"alert_id": req.ID,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "alert_rule.delete", "alert_rule", req.ID, nil)
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) testAlertRule(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		Query     map[string]any `json:"query"`
		Condition map[string]any `json:"condition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Condition == nil {
		writeError(w, http.StatusBadRequest, errors.New("alert condition is required"))
		return
	}
	condition, err := normalizeAlertCondition(req.Condition)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	query, err := a.decodeQueryPayload(req.Query, statePrincipal(r).TenantID, clickhouse.CustomSQLAlert)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := a.runAlertPreviewQuery(r, query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	evaluations := alert.EvaluateRows(condition, rows, "preview")
	if evaluations == nil {
		evaluations = []alert.Evaluation{}
	}
	value := 0.0
	if len(evaluations) > 0 {
		value = evaluations[0].Value
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"value":       value,
		"operator":    condition.Operator,
		"threshold":   condition.Threshold,
		"condition":   condition,
		"firing":      len(evaluations) > 0,
		"matches":     evaluations,
		"match_count": len(evaluations),
		"rows":        rows,
	})
}

func (a *App) validateQueryPayload(payload map[string]any, tenantID string, customMode clickhouse.CustomSQLMode) error {
	_, err := a.decodeQueryPayload(payload, tenantID, customMode)
	return err
}

func (a *App) decodeQueryPayload(payload map[string]any, tenantID string, customMode clickhouse.CustomSQLMode) (clickhouse.QueryRequest, error) {
	if payload == nil {
		return clickhouse.QueryRequest{}, errors.New("query payload is required")
	}
	queryBytes, err := json.Marshal(payload)
	if err != nil {
		return clickhouse.QueryRequest{}, err
	}
	var query clickhouse.QueryRequest
	if err := json.Unmarshal(queryBytes, &query); err != nil {
		return clickhouse.QueryRequest{}, err
	}
	ds, ok := a.cfg.Datasets[query.Dataset]
	if !ok {
		return clickhouse.QueryRequest{}, errors.New("unknown query dataset")
	}
	if query.Mode == "sql" {
		_, err = clickhouse.BuildCustomSQL(query, ds, tenantID, a.cfg.ClickHouse.MaxRows, customMode)
		return query, err
	}
	_, err = clickhouse.BuildTimeseriesSQL(query, ds, tenantID, a.cfg.ClickHouse.MaxRows)
	return query, err
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
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (a *App) runAlertPreviewQuery(r *http.Request, req clickhouse.QueryRequest) ([]map[string]any, error) {
	if req.Mode == "sql" {
		return a.runCustomSQL(r, req, clickhouse.CustomSQLAlert)
	}
	ds, ok := a.cfg.Datasets[req.Dataset]
	if !ok {
		return nil, errors.New("unknown dataset")
	}
	sql, err := clickhouse.BuildTimeseriesSQL(req, ds, statePrincipal(r).TenantID, a.cfg.ClickHouse.MaxRows)
	if err != nil {
		return nil, err
	}
	ch, err := a.clickHouseForQuery(r, req)
	if err != nil {
		return nil, err
	}
	return ch.QueryJSONEachRow(r.Context(), sql)
}

func normalizeAlertCondition(condition map[string]any) (alert.Condition, error) {
	conditionType, _ := condition["type"].(string)
	operator, _ := condition["operator"].(string)
	field, _ := condition["field"].(string)
	textValue, _ := condition["value"].(string)
	threshold, hasThreshold := numericValue(condition["threshold"])
	normalized := alert.NormalizeCondition(alert.Condition{
		Type:      strings.TrimSpace(conditionType),
		Operator:  strings.TrimSpace(operator),
		Field:     strings.TrimSpace(field),
		Threshold: threshold,
		Value:     textValue,
	})
	if alertConditionNeedsThreshold(normalized.Type) && !hasThreshold {
		return alert.Condition{}, errors.New("alert threshold must be numeric")
	}
	if !validAlertOperator(normalized.Type, normalized.Operator) {
		return alert.Condition{}, errors.New("alert operator is not supported")
	}
	if normalized.Type == "text_match" && strings.TrimSpace(normalized.Value) == "" {
		return alert.Condition{}, errors.New("alert text match value is required")
	}
	if rawFor, ok := condition["for"].(string); ok && strings.TrimSpace(rawFor) != "" {
		holdFor, err := time.ParseDuration(strings.TrimSpace(rawFor))
		if err != nil || holdFor < 0 {
			return alert.Condition{}, errors.New("alert for duration must be a valid duration such as 5m or 1h")
		}
		normalized.For = strings.TrimSpace(rawFor)
	}
	return normalized, nil
}

func applyNormalizedAlertCondition(target map[string]any, condition alert.Condition) {
	target["type"] = condition.Type
	target["operator"] = condition.Operator
	target["field"] = condition.Field
	target["threshold"] = condition.Threshold
	target["value"] = condition.Value
	if condition.For != "" {
		target["for"] = condition.For
	} else {
		delete(target, "for")
	}
}

func alertConditionNeedsThreshold(conditionType string) bool {
	switch conditionType {
	case "row_count", "numeric_threshold", "":
		return true
	default:
		return false
	}
}

func validAlertOperator(conditionType string, op string) bool {
	switch conditionType {
	case "text_match":
		switch op {
		case "contains", "not_contains", "eq", "neq", "regex":
			return true
		default:
			return false
		}
	case "any_rows", "sql_result", "no_data":
		return op != ""
	default:
		switch op {
		case "gt", "gte", "lt", "lte", "eq", "neq":
			return true
		default:
			return false
		}
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
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Target = strings.TrimSpace(req.Target)
	config, err := a.normalizeContactConfig(r, req.Kind, req.Name, req.Target, req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Kind == "pagerduty" && req.Target == "" {
		req.Target = "https://events.pagerduty.com/v2/enqueue"
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
	err = a.state.RPC(r.Context(), "save_contact_endpoint", map[string]any{
		"contact_id":     contactID,
		"contact_name":   req.Name,
		"contact_kind":   req.Kind,
		"contact_target": req.Target,
		"contact_config": config,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "contact_endpoint.save", "contact_endpoint", targetIDFromRows(rows), map[string]any{"name": req.Name, "kind": req.Kind})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) testContactEndpoint(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID     string         `json:"id"`
		Name   string         `json:"name"`
		Kind   string         `json:"kind"`
		Target string         `json:"target"`
		Config map[string]any `json:"config"`
		Action string         `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	contact, err := a.contactFromTestRequest(r, req.ID, req.Kind, req.Target, req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		action = "trigger"
	}
	if action == "validate" {
		result := a.validateContactForTest(r, contact)
		a.recordAuditEvent(r, "contact_endpoint.validate", "contact_endpoint", strings.TrimSpace(req.ID), map[string]any{"kind": contact.Kind, "status": result.Status})
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     result.Status,
			"statusCode": result.StatusCode,
			"error":      result.Error,
			"payload":    map[string]any{"validated": true, "kind": contact.Kind, "target": contact.Target},
		})
		return
	}
	tester := alert.NewDeliveryTester(alert.SMTPConfig{
		Host:     a.cfg.Alerts.SMTPHost,
		Port:     a.cfg.Alerts.SMTPPort,
		User:     a.cfg.Alerts.SMTPUser,
		Password: a.cfg.Alerts.SMTPPassword,
		From:     a.cfg.Alerts.SMTPFrom,
	}, a.resolveTenantSecretForRequest(r), a.logger)
	result, payload := tester.TestContactAction(r.Context(), statePrincipal(r).TenantID, contact, action)
	if _, err := a.state.RecordContactTestNotification(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), contact.Kind, contact.Target, result.Status, result.StatusCode, result.Error, payload); err != nil {
		a.logger.Warn("contact test notification record failed", "kind", contact.Kind, "target", contact.Target, "error", err)
	}
	a.recordAuditEvent(r, "contact_endpoint.test", "contact_endpoint", strings.TrimSpace(req.ID), map[string]any{"kind": contact.Kind, "status": result.Status})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     result.Status,
		"statusCode": result.StatusCode,
		"error":      result.Error,
		"payload":    payload,
	})
}

func (a *App) validateContactForTest(r *http.Request, contact alert.ContactEndpoint) alert.DeliveryResult {
	switch contact.Kind {
	case "email":
		if strings.TrimSpace(contact.Target) == "" || !strings.Contains(contact.Target, "@") {
			return alert.DeliveryResult{Status: "failed", Error: "email target is required"}
		}
		if strings.TrimSpace(a.cfg.Alerts.SMTPHost) == "" || strings.TrimSpace(a.cfg.Alerts.SMTPFrom) == "" {
			return alert.DeliveryResult{Status: "skipped", Error: "email delivery is not configured"}
		}
		return alert.DeliveryResult{Status: "success"}
	case "webhook":
		parsed, err := url.Parse(strings.TrimSpace(contact.Target))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return alert.DeliveryResult{Status: "failed", Error: "webhook target must be a valid URL"}
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			return alert.DeliveryResult{Status: "failed", Error: "webhook target must use http or https"}
		}
		if result := a.validateOptionalContactSecret(r, contact, "tokenSecretRef", "webhook bearer token"); result.Status != "success" {
			return result
		}
		if strings.TrimSpace(contact.Config["headerName"]) != "" {
			if !safeHTTPHeaderName(contact.Config["headerName"]) {
				return alert.DeliveryResult{Status: "failed", Error: "webhook header name is invalid"}
			}
			if result := a.validateOptionalContactSecret(r, contact, "headerValueSecretRef", "webhook header value"); result.Status != "success" {
				return result
			}
		}
		return alert.DeliveryResult{Status: "success"}
	case "pagerduty":
		if boolString(contact.Config["restSyncEnabled"]) {
			if strings.TrimSpace(contact.Config["serviceId"]) == "" {
				return alert.DeliveryResult{Status: "failed", Error: "PagerDuty REST service ID is required"}
			}
			if strings.TrimSpace(contact.Config["fromEmail"]) == "" {
				return alert.DeliveryResult{Status: "failed", Error: "PagerDuty REST From email is required"}
			}
			if value := strings.TrimSpace(contact.Config["apiBaseURL"]); value != "" {
				if err := validatePagerDutyRESTURL(value); err != nil {
					return alert.DeliveryResult{Status: "failed", Error: err.Error()}
				}
			}
			if result := a.validateOptionalContactSecret(r, contact, "restApiKeySecretRef", "PagerDuty REST API key"); result.Status != "success" {
				return result
			}
			return alert.DeliveryResult{Status: "success"}
		}
		if err := validatePagerDutyEventsURL(contact.Target); err != nil {
			return alert.DeliveryResult{Status: "failed", Error: err.Error()}
		}
		ref := strings.TrimSpace(contact.Config["routingKeySecretRef"])
		if ref == "" {
			return alert.DeliveryResult{Status: "failed", Error: "PagerDuty Events integration key secret is required"}
		}
		if _, ok := a.resolveTenantSecretForRequest(r)(r.Context(), statePrincipal(r).TenantID, ref); !ok {
			return alert.DeliveryResult{Status: "failed", Error: "PagerDuty Events integration key secret is not available: " + ref}
		}
		return alert.DeliveryResult{Status: "success"}
	default:
		return alert.DeliveryResult{Status: "failed", Error: fmt.Sprintf("unsupported contact kind: %s", contact.Kind)}
	}
}

func (a *App) validateOptionalContactSecret(r *http.Request, contact alert.ContactEndpoint, key string, label string) alert.DeliveryResult {
	ref := strings.TrimSpace(contact.Config[key])
	if ref == "" {
		return alert.DeliveryResult{Status: "success"}
	}
	if _, ok := a.resolveTenantSecretForRequest(r)(r.Context(), statePrincipal(r).TenantID, ref); !ok {
		return alert.DeliveryResult{Status: "failed", Error: label + " secret is not available: " + ref}
	}
	return alert.DeliveryResult{Status: "success"}
}

func (a *App) contactFromTestRequest(r *http.Request, id string, kind string, target string, config map[string]any) (alert.ContactEndpoint, error) {
	id = strings.TrimSpace(id)
	if id != "" && strings.TrimSpace(kind) == "" && strings.TrimSpace(target) == "" {
		var rows []map[string]any
		if err := a.state.RPC(r.Context(), "list_contact_endpoints", map[string]any{}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
			return alert.ContactEndpoint{}, err
		}
		for _, row := range rows {
			if fmt.Sprint(row["id"]) == id {
				return alert.ContactEndpoint{
					Kind:   fmt.Sprint(row["kind"]),
					Target: fmt.Sprint(row["target"]),
					Config: stringMapFromAny(row["config"]),
				}, nil
			}
		}
		return alert.ContactEndpoint{}, errors.New("contact endpoint was not found")
	}
	kind = strings.TrimSpace(kind)
	target = strings.TrimSpace(target)
	if kind == "pagerduty" && target == "" {
		target = "https://events.pagerduty.com/v2/enqueue"
	}
	if kind == "" || target == "" {
		return alert.ContactEndpoint{}, errors.New("contact kind and target are required")
	}
	if kind == "pagerduty" {
		if err := validatePagerDutyEventsURL(target); err != nil {
			return alert.ContactEndpoint{}, err
		}
	}
	return alert.ContactEndpoint{Kind: kind, Target: target, Config: stringMapFromAny(config)}, nil
}

func (a *App) resolveTenantSecretForRequest(r *http.Request) func(context.Context, string, string) (string, bool) {
	user := statePrincipal(r)
	bearer := r.Header.Get("Authorization")
	return func(ctx context.Context, tenantID string, secretName string) (string, bool) {
		if a.cfg.Secrets.EncryptionKey != "" && a.state.Enabled() {
			row, err := a.state.GetTenantSecret(ctx, user, bearer, strings.TrimSpace(secretName))
			if err == nil {
				value, decryptErr := secrets.DecryptString(row.Ciphertext, row.Nonce, a.cfg.Secrets.EncryptionKey)
				if decryptErr == nil {
					return value, true
				}
				a.logger.Warn("tenant secret decrypt failed", "tenant", tenantID, "secret", secretName, "error", decryptErr)
			}
		}
		return alert.ResolveSecretRefFromEnv(ctx, tenantID, secretName)
	}
}

func stringMapFromAny(input any) map[string]string {
	output := map[string]string{}
	if input == nil {
		return output
	}
	switch typed := input.(type) {
	case map[string]string:
		for key, value := range typed {
			output[key] = value
		}
	case map[string]any:
		for key, value := range typed {
			if text, ok := value.(string); ok {
				output[key] = strings.TrimSpace(text)
			}
		}
	}
	return output
}

func (a *App) normalizeContactConfig(r *http.Request, kind string, name string, target string, input map[string]any) (map[string]any, error) {
	switch kind {
	case "email", "webhook", "pagerduty":
	default:
		return nil, errors.New("contact kind is not supported")
	}
	output := map[string]any{}
	for key, value := range input {
		if text, ok := value.(string); ok {
			output[key] = strings.TrimSpace(text)
		}
	}
	if kind == "webhook" {
		return a.normalizeWebhookContactConfig(r, name, output)
	}
	if kind != "pagerduty" {
		return output, nil
	}
	if value, _ := output["routingKey"].(string); value != "" {
		return nil, errors.New("PagerDuty routing key must be stored as a secret ref, not inline config")
	}
	if value, _ := output["apiKey"].(string); value != "" {
		return nil, errors.New("PagerDuty API key must be stored as a secret ref, not inline config")
	}
	mode, _ := output["mode"].(string)
	if mode == "" {
		mode = "events_v2"
		output["mode"] = mode
	}
	if mode != "events_v2" {
		return nil, errors.New("PagerDuty REST incident sync is not implemented yet; use events_v2")
	}
	if target != "" {
		if err := validatePagerDutyEventsURL(target); err != nil {
			return nil, err
		}
	}
	if value, _ := output["routingKeyValue"].(string); value != "" {
		ref, _ := output["routingKeySecretRef"].(string)
		if ref == "" {
			ref = defaultSecretRef(name, "pagerduty-routing-key")
		}
		if err := a.saveEncryptedTenantSecret(r, "", ref, "PagerDuty Events API routing key", value); err != nil {
			return nil, err
		}
		output["routingKeySecretRef"] = ref
	}
	delete(output, "routingKeyValue")
	if value, _ := output["restApiKeyValue"].(string); value != "" {
		ref, _ := output["restApiKeySecretRef"].(string)
		if ref == "" {
			ref = defaultSecretRef(name, "pagerduty-rest-api-key")
		}
		if err := a.saveEncryptedTenantSecret(r, "", ref, "PagerDuty REST API key", value); err != nil {
			return nil, err
		}
		output["restApiKeySecretRef"] = ref
	}
	delete(output, "restApiKeyValue")
	restSyncEnabled := boolString(output["restSyncEnabled"])
	if restSyncEnabled {
		output["restSyncEnabled"] = "true"
		autoSyncEnabled := true
		if rawAutoSync, ok := output["autoSyncEnabled"]; ok {
			autoSyncEnabled = boolString(rawAutoSync)
		}
		if autoSyncEnabled {
			output["autoSyncEnabled"] = "true"
		} else {
			output["autoSyncEnabled"] = "false"
		}
		syncIntervalSeconds := 0
		if value, ok := numericValue(output["syncIntervalSeconds"]); ok {
			syncIntervalSeconds = int(value)
		} else if strings.TrimSpace(fmt.Sprint(output["syncIntervalSeconds"])) != "" {
			return nil, errors.New("PagerDuty auto sync interval must be a number of seconds")
		}
		if syncIntervalSeconds < 0 {
			return nil, errors.New("PagerDuty auto sync interval cannot be negative")
		}
		if syncIntervalSeconds > 0 && syncIntervalSeconds < 30 {
			return nil, errors.New("PagerDuty auto sync interval must be 0 or at least 30 seconds")
		}
		output["syncIntervalSeconds"] = fmt.Sprint(syncIntervalSeconds)
		if value, _ := output["restApiKeySecretRef"].(string); value == "" {
			return nil, errors.New("PagerDuty REST API key secret ref is required when REST sync is enabled")
		}
		if value, _ := output["serviceId"].(string); value == "" {
			return nil, errors.New("PagerDuty REST service ID is required when REST sync is enabled")
		}
		if value, _ := output["fromEmail"].(string); value == "" {
			return nil, errors.New("PagerDuty REST From email is required when REST sync is enabled")
		}
		if value, _ := output["apiBaseURL"].(string); value != "" {
			if err := validatePagerDutyRESTURL(value); err != nil {
				return nil, err
			}
		}
	} else if value, _ := output["routingKeySecretRef"].(string); value == "" {
		return nil, errors.New("PagerDuty Events integration key secret ref is required")
	}
	if value, _ := output["severity"].(string); value == "" {
		output["severity"] = "error"
	}
	return output, nil
}

func (a *App) normalizeWebhookContactConfig(r *http.Request, name string, output map[string]any) (map[string]any, error) {
	if value, _ := output["tokenValue"].(string); value != "" {
		ref, _ := output["tokenSecretRef"].(string)
		if ref == "" {
			ref = defaultSecretRef(name, "webhook-token")
		}
		if err := a.saveEncryptedTenantSecret(r, "", ref, "Webhook bearer token", value); err != nil {
			return nil, err
		}
		output["tokenSecretRef"] = ref
	}
	delete(output, "tokenValue")
	if headerName, _ := output["headerName"].(string); headerName != "" {
		if !safeHTTPHeaderName(headerName) {
			return nil, errors.New("webhook header name is invalid")
		}
		output["headerName"] = headerName
	}
	if value, _ := output["headerValue"].(string); value != "" {
		headerName, _ := output["headerName"].(string)
		if headerName == "" {
			return nil, errors.New("webhook header name is required when a header value is provided")
		}
		ref, _ := output["headerValueSecretRef"].(string)
		if ref == "" {
			ref = defaultSecretRef(name, headerName+"-header")
		}
		if err := a.saveEncryptedTenantSecret(r, "", ref, "Webhook custom header value", value); err != nil {
			return nil, err
		}
		output["headerValueSecretRef"] = ref
	}
	delete(output, "headerValue")
	return output, nil
}

func validatePagerDutyEventsURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("PagerDuty Events API URL must use https")
	}
	if parsed.Host != "events.pagerduty.com" {
		return errors.New("PagerDuty Events API URL must use events.pagerduty.com")
	}
	return nil
}

func validatePagerDutyRESTURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("PagerDuty REST API URL must use https")
	}
	if parsed.Host != "api.pagerduty.com" {
		return errors.New("PagerDuty REST API URL must use api.pagerduty.com")
	}
	return nil
}

func boolString(input any) bool {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(input)))
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

func (a *App) listTenantSecrets(w http.ResponseWriter, r *http.Request) {
	rows, err := a.state.ListTenantSecrets(r.Context(), statePrincipal(r), r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) saveTenantSecret(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Value       string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := a.saveEncryptedTenantSecretRows(r, strings.TrimSpace(req.ID), strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.recordAuditEvent(r, "tenant_secret.save", "tenant_secret", targetIDFromSecretRows(rows), map[string]any{"name": strings.TrimSpace(req.Name)})
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) deleteTenantSecret(w http.ResponseWriter, r *http.Request) {
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
	secret, ok, err := a.tenantSecretByID(r, strings.TrimSpace(req.ID))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if ok {
		usages, err := a.state.ListTenantSecretUsage(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), secret.Name)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if len(usages) > 0 {
			writeError(w, http.StatusConflict, fmt.Errorf("secret is still in use: %s", tenantSecretUsageSummary(usages)))
			return
		}
	}
	rows, err := a.state.DeleteTenantSecret(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), strings.TrimSpace(req.ID))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "tenant_secret.delete", "tenant_secret", strings.TrimSpace(req.ID), nil)
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) tenantSecretByID(r *http.Request, id string) (state.TenantSecret, bool, error) {
	if id == "" {
		return state.TenantSecret{}, false, nil
	}
	rows, err := a.state.ListTenantSecrets(r.Context(), statePrincipal(r), r.Header.Get("Authorization"))
	if err != nil {
		return state.TenantSecret{}, false, err
	}
	for _, row := range rows {
		if row.ID == id {
			return row, true, nil
		}
	}
	return state.TenantSecret{}, false, nil
}

func tenantSecretUsageSummary(usages []state.TenantSecretUsage) string {
	parts := make([]string, 0, len(usages))
	for _, usage := range usages {
		parts = append(parts, strings.TrimSpace(usage.ResourceName+" "+usage.Field))
	}
	return strings.Join(parts, ", ")
}

func (a *App) saveEncryptedTenantSecret(r *http.Request, id string, name string, description string, value string) error {
	_, err := a.saveEncryptedTenantSecretRows(r, id, name, description, value)
	return err
}

func (a *App) saveEncryptedTenantSecretRows(r *http.Request, id string, name string, description string, value string) ([]state.TenantSecret, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("secret name is required")
	}
	if value == "" {
		return nil, errors.New("secret value is required")
	}
	ciphertext, nonce, err := secrets.EncryptString(value, a.cfg.Secrets.EncryptionKey)
	if err != nil {
		return nil, err
	}
	return a.state.SaveTenantSecret(r.Context(), statePrincipal(r), r.Header.Get("Authorization"), id, name, description, ciphertext, nonce, secrets.KeyVersion)
}

func defaultSecretRef(name string, suffix string) string {
	base := strings.ToLower(strings.TrimSpace(name + "-" + suffix))
	base = strings.Trim(secretNameCleaner.ReplaceAllString(base, "-"), "-")
	if base == "" {
		return suffix
	}
	return base
}

var secretNameCleaner = regexp.MustCompile(`[^a-z0-9]+`)
var httpHeaderNamePattern = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

func safeHTTPHeaderName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && httpHeaderNamePattern.MatchString(name)
}

func targetIDFromSecretRows(rows []state.TenantSecret) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].ID
}

func (a *App) deleteContactEndpoint(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, errors.New("contact endpoint id is required"))
		return
	}
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "delete_contact_endpoint", map[string]any{
		"contact_id": req.ID,
	}, statePrincipal(r), r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "contact_endpoint.delete", "contact_endpoint", req.ID, nil)
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
	a.updateAlertIncidentStatus(w, r, "resolve_alert_incident", "resolve", "alert_incident.resolve")
}

func (a *App) acknowledgeAlertIncident(w http.ResponseWriter, r *http.Request) {
	a.updateAlertIncidentStatus(w, r, "acknowledge_alert_incident", "acknowledge", "alert_incident.acknowledge")
}

func (a *App) updateAlertIncidentStatus(w http.ResponseWriter, r *http.Request, rpcName string, pagerDutyAction string, auditAction string) {
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
	mappedPagerDutyIncident, hasPagerDutyMapping := a.findTenantPagerDutyIncident(r, req.ID)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), rpcName, map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
		"incident_id":    req.ID,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if hasPagerDutyMapping {
		tester := alert.NewDeliveryTester(alert.SMTPConfig{}, a.resolveTenantSecretForRequest(r), a.logger)
		result := tester.UpdatePagerDutyRemoteIncident(r.Context(), mappedPagerDutyIncident, pagerDutyAction)
		if _, err := a.state.UpdateAlertIncidentSync(r.Context(), a.cfg.Alerts.WorkerKey, mappedPagerDutyIncident.TenantID, mappedPagerDutyIncident.ID, result.ExternalProvider, result.ExternalIncidentID, result.ExternalIncidentURL, result.ExternalSyncStatus, result.Error); err != nil {
			a.logger.Warn("PagerDuty incident status sync record failed", "incident", req.ID, "action", pagerDutyAction, "error", err)
		}
	}
	a.recordAuditEvent(r, auditAction, "alert_incident", req.ID, nil)
	writeJSON(w, http.StatusOK, rows)
}

func (a *App) findTenantPagerDutyIncident(r *http.Request, incidentID string) (state.PagerDutySyncedIncident, bool) {
	user := statePrincipal(r)
	incident, ok, err := a.state.GetTenantPagerDutyIncident(r.Context(), user, r.Header.Get("Authorization"), incidentID)
	if err != nil {
		a.logger.Warn("PagerDuty mapped incident lookup failed", "incident", incidentID, "error", err)
		return state.PagerDutySyncedIncident{}, false
	}
	return incident, ok
}

func (a *App) syncPagerDutyIncidents(w http.ResponseWriter, r *http.Request) {
	if !a.requireStateRole(w, r, "owner", "admin", "editor") {
		return
	}
	user := statePrincipal(r)
	incidents, err := a.state.ListTenantPagerDutySyncedIncidents(r.Context(), user, r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	tester := alert.NewDeliveryTester(alert.SMTPConfig{}, a.resolveTenantSecretForRequest(r), a.logger)
	results := tester.ReconcilePagerDutyIncidents(r.Context(), incidents, func(ctx context.Context, incident state.PagerDutySyncedIncident, remote alert.PagerDutyRemoteIncident, result alert.DeliveryResult) error {
		status := remote.Status
		if result.Status == "failed" {
			status = ""
		}
		_, err := a.state.ReconcilePagerDutyIncident(ctx, a.cfg.Alerts.WorkerKey, incident.TenantID, incident.ID, status, result.ExternalIncidentURL, result.ExternalSyncStatus, result.Error)
		return err
	})
	a.recordAuditEvent(r, "pagerduty.sync", "alert_incident", "", map[string]any{"count": len(results)})
	loadErr := ""
	if len(incidents) == 0 {
		loadErr = "no mapped PagerDuty incidents found"
	}
	if results == nil {
		results = []alert.PagerDutyReconcileResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(results),
		"message": loadErr,
		"results": results,
	})
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

func (a *App) deleteInvite(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, errors.New("invite id is required"))
		return
	}
	user := statePrincipal(r)
	var rows []map[string]any
	if err := a.state.RPC(r.Context(), "delete_tenant_invite", map[string]any{
		"actor_subject":  user.Subject,
		"actor_provider": user.Provider,
		"invite_id":      req.ID,
	}, user, r.Header.Get("Authorization"), &rows); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.recordAuditEvent(r, "tenant_invite.delete", "tenant_invite", req.ID, nil)
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
	if _, err := a.syncCurrentUser(r, principal(r)); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return false
	}
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
		if activeTenant := strings.TrimSpace(r.Header.Get("X-SQViz-Tenant")); activeTenant != "" {
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
