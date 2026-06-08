package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"uvoo-dbviz/internal/auth"
	"uvoo-dbviz/internal/clickhouse"
	"uvoo-dbviz/internal/config"
	"uvoo-dbviz/internal/state"
)

func TestHealth(t *testing.T) {
	cfg := config.Load()
	app := New(cfg, auth.NewManager(cfg.Auth, nil, slog.Default()), clickhouse.NewClient(cfg.ClickHouse, nil), state.NewClient(cfg.PostgREST, nil), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
}
