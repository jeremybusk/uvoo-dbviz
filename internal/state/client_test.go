package state

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"uvoo-dbviz/internal/auth"
	"uvoo-dbviz/internal/config"
)

func TestRPCForwardsPrincipalHeadersAndBearer(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/rpc/current_user_has_role" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		assertHeader(t, r, "X-DBViz-Tenant", "dev")
		assertHeader(t, r, "X-DBViz-Subject", "alice")
		assertHeader(t, r, "X-DBViz-Provider", "keycloak")
		assertHeader(t, r, "X-DBViz-Email", "alice@example.com")
		assertHeader(t, r, "Authorization", "Bearer token")
		if r.Header.Get("X-Dev-Tenant") != "" {
			t.Fatalf("unexpected X-Dev-Tenant with bearer auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["user_subject"] != "alice" || body["user_provider"] != "keycloak" {
			t.Fatalf("unexpected body: %#v", body)
		}
		return textResponse(http.StatusOK, `true`), nil
	})}

	client := NewClient(config.PostgRESTConfig{URL: "http://postgrest:3000"}, httpClient)
	ok, err := client.CurrentUserHasRole(context.Background(), auth.Principal{
		TenantID: "dev",
		Subject:  "alice",
		Provider: "keycloak",
		Email:    "alice@example.com",
	}, "Bearer token", []string{"viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected role check to return true")
	}
}

func TestRPCAddsDevHeadersWhenBearerIsMissing(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assertHeader(t, r, "X-DBViz-Tenant", "dev")
		assertHeader(t, r, "X-Dev-Tenant", "dev")
		assertHeader(t, r, "X-Dev-Email", "dev@example.com")
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected Authorization header")
		}
		return textResponse(http.StatusOK, `true`), nil
	})}

	client := NewClient(config.PostgRESTConfig{URL: "http://postgrest:3000"}, httpClient)
	ok, err := client.CurrentUserHasRole(context.Background(), auth.Principal{
		TenantID: "dev",
		Subject:  "dev@example.com",
		Provider: "dev",
		Email:    "dev@example.com",
	}, "", []string{"owner"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected role check to return true")
	}
}

func assertHeader(t *testing.T, r *http.Request, name, want string) {
	t.Helper()
	if got := r.Header.Get(name); got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
