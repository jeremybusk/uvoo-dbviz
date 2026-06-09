package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"uvoo-dbviz/internal/config"
)

type Principal struct {
	Subject  string            `json:"subject"`
	TenantID string            `json:"tenantId"`
	Email    string            `json:"email"`
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Claims   map[string]any    `json:"claims,omitempty"`
	Headers  map[string]string `json:"-"`
}

type Manager struct {
	cfg    config.AuthConfig
	http   *http.Client
	logger *slog.Logger
	mu     sync.Mutex
	cache  map[string]discoveryCache
}

type discoveryCache struct {
	value     discovery
	expiresAt time.Time
}

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type tokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func NewManager(cfg config.AuthConfig, client *http.Client, logger *slog.Logger) *Manager {
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{cfg: cfg, http: client, logger: logger, cache: map[string]discoveryCache{}}
}

func (m *Manager) PublicDiscovery(ctx context.Context, providerID string) (map[string]string, error) {
	provider, ok := m.provider(providerID)
	if !ok || !provider.Enabled {
		return nil, errors.New("provider is not enabled")
	}
	d, err := m.discover(ctx, provider)
	if err != nil {
		return nil, err
	}
	d = publicDiscovery(provider, d)
	return map[string]string{
		"authorizationEndpoint": d.AuthorizationEndpoint,
		"tokenEndpoint":         d.TokenEndpoint,
		"issuer":                d.Issuer,
	}, nil
}

func (m *Manager) ExchangeCode(ctx context.Context, providerID, code, redirectURI, verifier string) (tokenResponse, error) {
	provider, ok := m.provider(providerID)
	if !ok || !provider.Enabled {
		return tokenResponse{}, errors.New("provider is not enabled")
	}
	if provider.ClientID == "" {
		return tokenResponse{}, errors.New("provider client id is not configured")
	}
	d, err := m.discover(ctx, provider)
	if err != nil {
		return tokenResponse{}, err
	}
	d = internalDiscovery(provider, d)
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", provider.ClientID)
	form.Set("code_verifier", verifier)
	if provider.ClientSecret != "" {
		form.Set("client_secret", provider.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.http.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("token endpoint returned %s: %s", resp.Status, string(body))
	}
	var tokens tokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return tokenResponse{}, err
	}
	return tokens, nil
}

func (m *Manager) Authenticate(r *http.Request) (Principal, error) {
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return m.VerifyJWT(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
	}
	if m.cfg.DevMode {
		tenant := strings.TrimSpace(r.Header.Get("X-Dev-Tenant"))
		if tenant == "" {
			tenant = "dev"
		}
		email := strings.TrimSpace(r.Header.Get("X-Dev-Email"))
		if email == "" {
			email = "dev@example.local"
		}
		return Principal{Subject: email, TenantID: tenant, Email: email, Name: "Development User", Provider: "dev"}, nil
	}
	return Principal{}, errors.New("missing bearer token")
}

func (m *Manager) VerifyJWT(ctx context.Context, token string) (Principal, error) {
	header, claims, signingInput, sig, err := splitJWT(token)
	if err != nil {
		return Principal{}, err
	}
	issuer, _ := claims["iss"].(string)
	provider, ok := m.providerForIssuer(issuer)
	if !ok || !provider.Enabled {
		return Principal{}, fmt.Errorf("issuer is not trusted: %s", issuer)
	}
	if !validAudience(claims["aud"], provider.Audience) {
		return Principal{}, errors.New("token audience is not accepted")
	}
	if err := validTimes(claims); err != nil {
		return Principal{}, err
	}
	d, err := m.discover(ctx, provider)
	if err != nil {
		return Principal{}, err
	}
	d = internalDiscovery(provider, d)
	keys, err := m.fetchJWKS(ctx, d.JWKSURI)
	if err != nil {
		return Principal{}, err
	}
	if err := verifySignature(header, signingInput, sig, keys); err != nil {
		return Principal{}, err
	}
	email := claimString(claims, provider.EmailClaim)
	name := claimString(claims, provider.NameClaim)
	tenant := claimString(claims, provider.TenantClaim)
	if tenant == "" {
		tenant = tenantFromEmail(email)
	}
	if tenant == "" {
		return Principal{}, errors.New("token does not contain a tenant claim or email domain")
	}
	if len(provider.AllowedDomains) > 0 && !domainAllowed(email, provider.AllowedDomains) {
		return Principal{}, errors.New("email domain is not allowed for provider")
	}
	return Principal{
		Subject:  claimString(claims, "sub"),
		TenantID: tenant,
		Email:    email,
		Name:     name,
		Provider: provider.ID,
		Claims:   claims,
	}, nil
}

func (m *Manager) provider(id string) (config.OIDCProvider, bool) {
	for _, provider := range m.cfg.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return config.OIDCProvider{}, false
}

func (m *Manager) providerForIssuer(issuer string) (config.OIDCProvider, bool) {
	for _, provider := range m.cfg.Providers {
		if provider.Issuer == issuer {
			return provider, true
		}
		for _, prefix := range provider.AllowIssuerPrefixes {
			if strings.HasPrefix(issuer, prefix) {
				return provider, true
			}
		}
	}
	return config.OIDCProvider{}, false
}

func (m *Manager) discover(ctx context.Context, provider config.OIDCProvider) (discovery, error) {
	discoveryURL := providerDiscoveryURL(provider)
	m.mu.Lock()
	if cached, ok := m.cache[discoveryURL]; ok && time.Now().Before(cached.expiresAt) {
		m.mu.Unlock()
		return cached.value, nil
	}
	m.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(discoveryURL, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return discovery{}, err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return discovery{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return discovery{}, fmt.Errorf("oidc discovery returned %s", resp.Status)
	}
	var d discovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return discovery{}, err
	}
	m.mu.Lock()
	m.cache[discoveryURL] = discoveryCache{value: d, expiresAt: time.Now().Add(15 * time.Minute)}
	m.mu.Unlock()
	return d, nil
}

func providerDiscoveryURL(provider config.OIDCProvider) string {
	if strings.TrimSpace(provider.DiscoveryURL) != "" {
		return strings.TrimSpace(provider.DiscoveryURL)
	}
	return provider.Issuer
}

func publicDiscovery(provider config.OIDCProvider, d discovery) discovery {
	return rewriteDiscoveryBase(d, providerDiscoveryURL(provider), provider.Issuer)
}

func internalDiscovery(provider config.OIDCProvider, d discovery) discovery {
	return rewriteDiscoveryBase(d, provider.Issuer, providerDiscoveryURL(provider))
}

func rewriteDiscoveryBase(d discovery, fromBase, toBase string) discovery {
	fromBase = strings.TrimRight(strings.TrimSpace(fromBase), "/")
	toBase = strings.TrimRight(strings.TrimSpace(toBase), "/")
	if fromBase == "" || toBase == "" || fromBase == toBase {
		return d
	}
	d.Issuer = rewriteURLBase(d.Issuer, fromBase, toBase)
	d.AuthorizationEndpoint = rewriteURLBase(d.AuthorizationEndpoint, fromBase, toBase)
	d.TokenEndpoint = rewriteURLBase(d.TokenEndpoint, fromBase, toBase)
	d.JWKSURI = rewriteURLBase(d.JWKSURI, fromBase, toBase)
	return d
}

func rewriteURLBase(value, fromBase, toBase string) string {
	if strings.HasPrefix(value, fromBase) {
		return toBase + strings.TrimPrefix(value, fromBase)
	}
	return value
}

func (m *Manager) fetchJWKS(ctx context.Context, uri string) (jwks, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return jwks{}, err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return jwks{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return jwks{}, fmt.Errorf("jwks endpoint returned %s", resp.Status)
	}
	var keys jwks
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&keys); err != nil {
		return jwks{}, err
	}
	return keys, nil
}

func splitJWT(token string) (map[string]any, map[string]any, []byte, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, nil, nil, errors.New("invalid jwt format")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, nil, nil, err
	}
	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var header, claims map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return nil, nil, nil, nil, err
	}
	return header, claims, []byte(parts[0] + "." + parts[1]), sig, nil
}

func verifySignature(header map[string]any, signingInput, sig []byte, keys jwks) error {
	kid := claimString(header, "kid")
	alg := claimString(header, "alg")
	for _, key := range keys.Keys {
		if kid != "" && key.Kid != kid {
			continue
		}
		if key.Alg != "" && alg != "" && key.Alg != alg {
			continue
		}
		if err := verifyWithKey(alg, signingInput, sig, key); err == nil {
			return nil
		}
	}
	return errors.New("jwt signature verification failed")
}

func verifyWithKey(alg string, signingInput, sig []byte, key jwk) error {
	switch alg {
	case "RS256":
		pub, err := rsaPublicKey(key)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(signingInput)
		return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
	case "ES256":
		pub, err := ecdsaPublicKey(key)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(signingInput)
		return verifyECDSA(pub, sum[:], sig)
	case "RS384":
		pub, err := rsaPublicKey(key)
		if err != nil {
			return err
		}
		sum := sha512.Sum384(signingInput)
		return rsa.VerifyPKCS1v15(pub, crypto.SHA384, sum[:], sig)
	case "RS512":
		pub, err := rsaPublicKey(key)
		if err != nil {
			return err
		}
		sum := sha512.Sum512(signingInput)
		return rsa.VerifyPKCS1v15(pub, crypto.SHA512, sum[:], sig)
	default:
		return fmt.Errorf("unsupported jwt alg: %s", alg)
	}
}

func rsaPublicKey(key jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func ecdsaPublicKey(key jwk) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(key.Y)
	if err != nil {
		return nil, err
	}
	curve := elliptic.P256()
	if key.Crv != "" && key.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported ec curve: %s", key.Crv)
	}
	return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xBytes), Y: new(big.Int).SetBytes(yBytes)}, nil
}

func verifyECDSA(pub *ecdsa.PublicKey, digest, sig []byte) error {
	if len(sig) != 64 {
		return errors.New("invalid ecdsa signature size")
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest, r, s) {
		return errors.New("invalid ecdsa signature")
	}
	return nil
}

func validAudience(value any, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	switch aud := value.(type) {
	case string:
		return contains(allowed, aud)
	case []any:
		for _, item := range aud {
			if s, ok := item.(string); ok && contains(allowed, s) {
				return true
			}
		}
	}
	return false
}

func validTimes(claims map[string]any) error {
	now := time.Now().Unix()
	if exp, ok := numberClaim(claims["exp"]); ok && int64(exp) < now-60 {
		return errors.New("token is expired")
	}
	if nbf, ok := numberClaim(claims["nbf"]); ok && int64(nbf) > now+60 {
		return errors.New("token is not valid yet")
	}
	return nil
}

func claimString(claims map[string]any, key string) string {
	if key == "" {
		return ""
	}
	if value, ok := claims[key].(string); ok {
		return value
	}
	return ""
}

func numberClaim(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

func tenantFromEmail(email string) string {
	_, domain, ok := strings.Cut(email, "@")
	if !ok {
		return ""
	}
	return strings.ToLower(domain)
}

func domainAllowed(email string, domains []string) bool {
	domain := tenantFromEmail(email)
	for _, allowed := range domains {
		if strings.EqualFold(domain, allowed) {
			return true
		}
	}
	return false
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
