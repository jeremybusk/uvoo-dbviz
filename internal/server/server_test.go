package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"uvoo-sqviz/internal/auth"
	"uvoo-sqviz/internal/clickhouse"
	"uvoo-sqviz/internal/config"
	"uvoo-sqviz/internal/state"
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

func TestStatePrincipalUsesActiveTenantHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := withPrincipal(req.Context(), auth.Principal{
		Subject:  "alice",
		TenantID: "home",
		Headers:  map[string]string{"ActiveTenantID": "selected"},
	})
	req = req.WithContext(ctx)

	user := statePrincipal(req)
	if user.TenantID != "selected" {
		t.Fatalf("tenant = %q, want selected", user.TenantID)
	}
}

func TestStatePrincipalFallsBackToAuthenticatedTenant(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req = req.WithContext(withPrincipal(context.Background(), auth.Principal{
		Subject:  "alice",
		TenantID: "home",
	}))

	user := statePrincipal(req)
	if user.TenantID != "home" {
		t.Fatalf("tenant = %q, want home", user.TenantID)
	}
}
