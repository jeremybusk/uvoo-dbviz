package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"uvoo-sqviz/internal/config"
)

func TestVerifyJWTUsesInternalDiscoveryAndPublicIssuer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const publicIssuer = "http://localhost:8089/realms/sqviz"
	const internalIssuer = "http://keycloak:8080/realms/sqviz"

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/realms/sqviz/.well-known/openid-configuration":
			return testJSONResponse(t, discovery{
				Issuer:                internalIssuer,
				AuthorizationEndpoint: internalIssuer + "/protocol/openid-connect/auth",
				TokenEndpoint:         internalIssuer + "/protocol/openid-connect/token",
				JWKSURI:               internalIssuer + "/protocol/openid-connect/certs",
			})
		case "/realms/sqviz/protocol/openid-connect/certs":
			return testJSONResponse(t, jwks{Keys: []jwk{rsaTestJWK(key.PublicKey, "test-key")}})
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     http.Header{},
			}, nil
		}
	})}

	manager := NewManager(config.AuthConfig{Providers: []config.OIDCProvider{{
		ID:           "keycloak",
		Name:         "Keycloak",
		Issuer:       publicIssuer,
		DiscoveryURL: internalIssuer,
		ClientID:     "uvoo-sqviz-web",
		Audience:     []string{"uvoo-sqviz-web"},
		TenantClaim:  "tenant_slug",
		EmailClaim:   "email",
		NameClaim:    "name",
		Enabled:      true,
	}}}, client, slog.Default())

	public, err := manager.PublicDiscovery(context.Background(), "keycloak")
	if err != nil {
		t.Fatal(err)
	}
	if got := public["authorizationEndpoint"]; !strings.HasPrefix(got, publicIssuer) {
		t.Fatalf("authorization endpoint was not rewritten to public issuer: %s", got)
	}

	token := signedJWT(t, key, map[string]any{
		"iss":         publicIssuer,
		"sub":         "alice",
		"aud":         "uvoo-sqviz-web",
		"exp":         time.Now().Add(time.Hour).Unix(),
		"tenant_slug": "dev",
		"email":       "alice@example.com",
		"name":        "Alice",
	})
	principal, err := manager.VerifyJWT(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Subject != "alice" || principal.TenantID != "dev" || principal.Provider != "keycloak" {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func testJSONResponse(t *testing.T, value any) (*http.Response, error) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(payload))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func signedJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": "test-key", "typ": "JWT"}
	signingInput := base64JSON(t, header) + "." + base64JSON(t, claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func base64JSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func rsaTestJWK(key rsa.PublicKey, kid string) jwk {
	return jwk{
		Kty: "RSA",
		Use: "sig",
		Kid: kid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
