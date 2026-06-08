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
)

type App struct {
	cfg    config.Config
	authn  *auth.Manager
	ch     *clickhouse.Client
	logger *slog.Logger
	mux    *http.ServeMux
}

func New(cfg config.Config, authn *auth.Manager, ch *clickhouse.Client, logger *slog.Logger) http.Handler {
	app := &App{cfg: cfg, authn: authn, ch: ch, logger: logger, mux: http.NewServeMux()}
	app.routes()
	return securityHeaders(app.mux)
}

func (a *App) routes() {
	a.mux.HandleFunc("GET /healthz", a.health)
	a.mux.HandleFunc("GET /api/config", a.publicConfig)
	a.mux.HandleFunc("GET /api/oidc/{provider}/discovery", a.oidcDiscovery)
	a.mux.HandleFunc("POST /api/oidc/{provider}/exchange", a.oidcExchange)
	a.mux.HandleFunc("GET /api/me", a.requireAuth(a.me))
	a.mux.HandleFunc("POST /api/query", a.requireAuth(a.query))
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
